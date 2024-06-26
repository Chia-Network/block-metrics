package metrics

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/chia-network/go-chia-libs/pkg/bech32m"
	"github.com/chia-network/go-chia-libs/pkg/rpc"
	"github.com/chia-network/go-chia-libs/pkg/types"
	"github.com/schollz/progressbar/v3"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

// BackfillBlocks loads all the blocks from the chia full node and stores the relevant data into the metrics DB
func (m *Metrics) BackfillBlocks() error {
	var (
		oldestHeight uint32
	)

	// We will start with either the oldest block in the DB, or the blockchain peak height, if the DB is empty
	oldestRow := m.mysqlClient.QueryRow("select height from blocks order by height asc limit 1")
	err := oldestRow.Scan(&oldestHeight)
	if err != nil {
		state, _, err := m.websocketClient.FullNodeService.GetBlockchainState()
		if err != nil {
			log.Fatalf("Error getting blockchain state: %s\n", err.Error())
		}
		if state.BlockchainState.IsAbsent() || state.BlockchainState.MustGet().Peak.IsAbsent() {
			return fmt.Errorf("blockchain state or peak not present in the response")
		}

		oldestHeight = state.BlockchainState.MustGet().Peak.MustGet().Height
	}

	start := oldestHeight - m.rpcPerPage
	end := oldestHeight

	bar := progressbar.Default(int64(oldestHeight))
	for {
		err = m.fetchAndSaveBlocksBetween(start, end)
		if err != nil {
			return err
		}

		err = bar.Add(int(m.rpcPerPage))
		_ = err // Just the progress bar, so it's not critical

		if start <= 0 {
			err = bar.Finish()
			_ = err // Just the progress bar, so it's not critical
			break
		}
		if start <= m.rpcPerPage {
			start = 0
		} else {
			start = start - m.rpcPerPage
		}
		end = end - m.rpcPerPage

		// Fills any missing timestamps
		err = m.FillTimestampGaps()
		if err != nil {
			return err
		}
	}

	return nil
}

func (m *Metrics) fetchAndSaveBlocksBetween(start, end uint32) error {
	blocks, _, err := m.websocketClient.FullNodeService.GetBlocks(&rpc.GetBlocksOptions{
		Start:          int(start),
		End:            int(end),
		ExcludeReorged: true,
	})
	if err != nil {
		return err
	}

	// Write to DB
	if blocks.Blocks.IsAbsent() {
		return fmt.Errorf("unable to fetch batch of blocks")
	}

	for _, block := range blocks.Blocks.MustGet() {
		err = m.saveBlock(block)
		if err != nil {
			return err
		}
	}

	return nil
}

// FillBlockGaps looks for gaps in the blocks table and fetches the missing blocks
// Avoids anything below the lowest block currently in the table
// We work from lowest height to the highest height, so that we can always be sure the preceding transaction block
// is present before the non-tx blocks that follow it, so that we can borrow the timestamp from the TX block
func (m *Metrics) FillBlockGaps() error {
	m.fillGapsLock.Lock()
	defer m.fillGapsLock.Unlock()
	query := "SELECT (t1.height + 1) as gap_starts_at, " +
		"       (SELECT MIN(t3.height) -1 FROM blocks t3 WHERE t3.height > t1.height) as gap_ends_at " +
		"FROM blocks t1 " +
		"WHERE NOT EXISTS (SELECT t2.height FROM blocks t2 WHERE t2.height = t1.height + 1) " +
		"HAVING gap_ends_at IS NOT NULL"

	rows, err := m.mysqlClient.Query(query)
	if err != nil {
		return err
	}
	defer func(rows *sql.Rows) {
		err := rows.Close()
		if err != nil {
			log.Errorf("Could not close rows: %s\n", err.Error())
		}
	}(rows)

	var (
		start uint32
		end   uint32
	)

	startEnd := map[uint32]uint32{}

	for rows.Next() {
		err = rows.Scan(&start, &end)
		if err != nil {
			return err
		}

		startEnd[start] = end
	}

	// Sort starting blocks lowest to highest, so we can properly fill timestamps
	keys := make([]uint32, 0, len(startEnd))
	for k := range startEnd {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		return i < j
	})

	// Fill blocks
	for _, startBlock := range keys {
		// Ensure we don't request too many blocks at once and kill the node RPC
		endBlock := startEnd[startBlock] + 1 // end is not inclusive in this func, so adding 1
		start := endBlock - m.rpcPerPage

		for {
			if start < startBlock {
				start = startBlock
			}
			log.Printf("Fetching blocks between %d and %d\n", start, endBlock)
			err = m.fetchAndSaveBlocksBetween(start, endBlock)
			if err != nil {
				return err
			}

			if start <= startBlock {
				break
			}

			start = start - m.rpcPerPage
			endBlock = endBlock - m.rpcPerPage

			// Fills any missing timestamps
			err = m.FillTimestampGaps()
			if err != nil {
				return err
			}
		}
	}

	return m.FillTimestampGaps()
}

// FillTimestampGaps In some cases, there might be blocks that for one reason or another, dont have a timestamp associated
// This identifies those gaps, and adds the missing timestamps
func (m *Metrics) FillTimestampGaps() error {
	query := "select height from blocks where timestamp IS NULL order by height asc;"

	var (
		height uint32
	)

	rows, err := m.mysqlClient.Query(query)
	if err != nil {
		return err
	}

	for rows.Next() {
		err = rows.Scan(&height)
		if err != nil {
			return err
		}

		timestamp := m.GetNonTXBlockTimestamp(height)
		insert, err := m.mysqlClient.Query("UPDATE blocks set timestamp=? where height=?;", timestamp, height)
		if err != nil {
			return err
		}
		err = insert.Close()
		if err != nil {
			return err
		}
	}

	err = rows.Close()
	if err != nil {
		return err
	}

	return nil
}

// receiveBlock is the callback when we receive a block via a websocket subscription
func (m *Metrics) receiveBlock(resp *types.WebsocketResponse) {
	block := &types.BlockEvent{}
	err := json.Unmarshal(resp.Data, block)
	if err != nil {
		log.Errorf("Error unmarshalling: %s\n", err.Error())
		return
	}

	if block.ReceiveBlockResult.OrElse(types.ReceiveBlockResultInvalidBlock) == types.ReceiveBlockResultNewPeak {
		log.Printf("Received block %d\n", block.Height)

		// The block event doesn't actually have the full block record, so grab it from the RPC
		result, _, err := m.websocketClient.FullNodeService.GetBlockByHeight(&rpc.GetBlockByHeightOptions{BlockHeight: int(block.Height)})
		if err != nil {
			log.Errorf("Error getting block in response to webhook: %s\n", err.Error())
			return
		}

		// result can sometimes be nil when the websocket message about a new block is from a node that is slightly
		// ahead of the node that we request the block from
		// In these cases, it should be relatively safe to skip the block, since the automatic backfill should
		// get the block later
		if result == nil || result.Block.IsAbsent() {
			log.Errorf("Block was not present in the response")
			return
		}

		err = m.saveBlock(result.Block.MustGet())
		if err != nil {
			log.Errorf("Error saving block: %s\n", err.Error())
			return
		}

		m.refreshMetrics(block.Height)
	}
}

// GetNonTXBlockTimestamp returns a timestamp to use for a non-transaction block. Returns the timestamp from the next
// lowest block that has a timestamp
// This relies on processing blocks from oldest to the newest
// The only case where we DONT process blocks in this order is the backfill --delete-first option, which goes backwards,
// so there is useful data ASAP
// For this case, the "fill missing timestamps" will catch and resolve the issue
func (m *Metrics) GetNonTXBlockTimestamp(blockHeight uint32) sql.NullString {
	query := "select timestamp from blocks " +
		"where height < ? " +
		"and height > ? " +
		"and timestamp IS NOT NULL order by height desc limit 1;"

	// Constrain to 10 blocks older to make sure we aren't accidentally getting a very old timestamp
	// Typically this is 5 or less from my observations, but this just allows a buffer, just in case
	rows, err := m.mysqlClient.Query(query, blockHeight, blockHeight-10)
	if err != nil {
		return sql.NullString{}
	}
	defer func(rows *sql.Rows) {
		err := rows.Close()
		if err != nil {
			log.Errorf("Could not close rows: %s\n", err.Error())
		}
	}(rows)

	var (
		timestr string
	)
	rows.Next()
	err = rows.Scan(&timestr)
	if err != nil {
		return sql.NullString{}
	}

	return sql.NullString{
		String: timestr,
		Valid:  true,
	}
}

func (m *Metrics) saveBlock(block types.FullBlock) error {
	blockHeight := block.RewardChainBlock.Height
	farmerPuzzHash := block.Foliage.FoliageBlockData.FarmerRewardPuzzleHash.String()
	farmerAddress, _ := bech32m.EncodePuzzleHash(block.Foliage.FoliageBlockData.FarmerRewardPuzzleHash, "xch")

	var timestamp sql.NullString
	if block.FoliageTransactionBlock.IsPresent() {
		timestamp = sql.NullString{
			String: block.FoliageTransactionBlock.MustGet().Timestamp.Format("2006-01-02 15:04:05"),
			Valid:  true,
		}
	} else {
		timestamp = m.GetNonTXBlockTimestamp(blockHeight)
	}
	insert, err := m.mysqlClient.Query("INSERT INTO blocks (timestamp, height, transaction_block, farmer_puzzle_hash, farmer_address) VALUES(?, ?, ?, ?, ?)", timestamp, blockHeight, block.FoliageTransactionBlock.IsPresent(), farmerPuzzHash, farmerAddress)
	if err != nil {
		return err
	}
	err = insert.Close()
	if err != nil {
		return err
	}

	return nil
}

// refreshMetrics updates the metrics using the provided peak height as a starting point to look back from
func (m *Metrics) refreshMetrics(peakHeight uint32) {
	// Update the highest block we've seen, if this is larger
	m.peakLock.Lock()
	if peakHeight <= m.highestPeak {
		return
	}
	m.highestPeak = peakHeight
	m.peakLock.Unlock()

	// Now wait until nothing else is refreshing metrics
	m.refreshing.Lock()
	defer m.refreshing.Unlock()

	// Now that we can process the metrics, one last check to make sure its still the highest peak we've seen
	if peakHeight < m.highestPeak {
		return
	}

	err := m.FillBlockGaps()
	if err != nil {
		log.Errorf("error backfilling gaps: %s\n", err.Error())
		return
	}

	nakamoto50, err := m.CalculateNakamoto(peakHeight, 50, []string{})
	if err != nil {
		log.Errorf("Error calculating 50%% threshold nakamoto coefficient: %s\n", err.Error())
		return
	}

	nakamoto51, err := m.CalculateNakamoto(peakHeight, 51, []string{})
	if err != nil {
		log.Errorf("Error calculating 51%% threshold nakamoto coefficient: %s\n", err.Error())
		return
	}

	nakamoto50Adj, err := m.CalculateNakamoto(peakHeight, 50, viper.GetStringSlice("adjusted-ignore-addresses"))
	if err != nil {
		log.Errorf("Error calculating 50%% threshold adjusted nakamoto coefficient: %s\n", err.Error())
		return
	}

	nakamoto51Adj, err := m.CalculateNakamoto(peakHeight, 51, viper.GetStringSlice("adjusted-ignore-addresses"))
	if err != nil {
		log.Errorf("Error calculating 51%% threshold adjusted nakamoto coefficient: %s\n", err.Error())
		return
	}

	m.prometheusMetrics.nakamotoCoefficient50.Set(float64(nakamoto50))
	m.prometheusMetrics.nakamotoCoefficient51.Set(float64(nakamoto51))
	m.prometheusMetrics.nakamotoCoefficient50Adjusted.Set(float64(nakamoto50Adj))
	m.prometheusMetrics.nakamotoCoefficient51Adjusted.Set(float64(nakamoto51Adj))
	m.prometheusMetrics.blockHeight.Set(float64(peakHeight))
}

// CalculateNakamoto calculates the NC for the given peak height and percentage
func (m *Metrics) CalculateNakamoto(peakHeight uint32, thresholdPercent int, ignoreAddresses []string) (int, error) {
	lookbackWindowPercent := float64(m.lookbackWindow) / 100
	minHeight := peakHeight - m.lookbackWindow

	// First, make sure we actually have enough blocks in the lookback window to do accurate math
	// Otherwise, just return an error (assume we are still syncing block history over)
	countQuery := "select count(*) from blocks where height > ?"
	countRow := m.mysqlClient.QueryRow(countQuery, minHeight)
	var count uint32
	err := countRow.Scan(&count)
	if err != nil {
		return 0, err
	}
	if count < m.lookbackWindow {
		return 0, fmt.Errorf("do not have %d blocks in database to use for nakamoto coefficient calculation", m.lookbackWindow)
	}

	//if ignoreAddresses is nothing, just add an empty string
	if len(ignoreAddresses) == 0 {
		ignoreAddresses = append(ignoreAddresses, "")
	}
	query := "select number, cumulative_percent from ( " +
		"select " +
		"        row_number() over (order by count(*) desc, farmer_address asc) as number, " +
		"        farmer_address, " +
		"        count(*) as count, " +
		"        sum(count(*)) over (order by count(*) desc, farmer_address asc) as cumulative_count, " +
		"        count(*)/? as percent, " +
		"        sum(count(*)) over (order by count(*) desc, farmer_address asc) / ? as cumulative_percent " +
		"    from blocks where height > ? and height <= ? and farmer_address NOT IN (?" + strings.Repeat(",?", len(ignoreAddresses)-1) + ") group by farmer_address order by count DESC, farmer_address ASC limit 1000 " +
		") as intermediary " +
		"where cumulative_percent >= ? order by cumulative_percent asc, number asc limit 1;"
	// 1: lookbackWindowPercent
	// 2: lookbackWindowPercent
	// 3: minHeight
	// 4: peakHeight
	// 5: Ignore Addresses...
	// 6: thresholdPercent
	args := []interface{}{lookbackWindowPercent, lookbackWindowPercent, minHeight, peakHeight}
	for _, _ignore := range ignoreAddresses {
		args = append(args, _ignore)
	}
	args = append(args, thresholdPercent)
	row := m.mysqlClient.QueryRow(query, args...)

	var (
		number            int
		cumulativePercent float64
	)
	err = row.Scan(&number, &cumulativePercent)
	if err != nil {
		return 0, err
	}

	return number, nil
}

// GetOldestBlock returns the oldest block height from the DB
func (m *Metrics) GetOldestBlock() (uint32, error) {
	countQuery := "select height from blocks order by height asc limit 1"
	countRow := m.mysqlClient.QueryRow(countQuery)
	var height uint32
	err := countRow.Scan(&height)
	if err != nil {
		return 0, err
	}

	return height, nil
}

// GetNewestBlock returns the newest block height from the DB
func (m *Metrics) GetNewestBlock() (uint32, error) {
	countQuery := "select height from blocks order by height desc limit 1"
	countRow := m.mysqlClient.QueryRow(countQuery)
	var height uint32
	err := countRow.Scan(&height)
	if err != nil {
		return 0, err
	}

	return height, nil
}
