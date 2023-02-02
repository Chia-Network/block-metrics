package metrics

import (
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/chia-network/go-chia-libs/pkg/bech32m"
	"github.com/chia-network/go-chia-libs/pkg/rpc"
	"github.com/chia-network/go-chia-libs/pkg/types"
	"github.com/schollz/progressbar/v3"
	log "github.com/sirupsen/logrus"
)

// BackfillBlocks loads all the blocks from the chia full node and stores the relevant data into the metrics DB
func (m *Metrics) BackfillBlocks() error {
	state, _, err := m.httpClient.FullNodeService.GetBlockchainState()
	if err != nil {
		log.Fatalf("Error getting blockchain state: %s\n", err.Error())
	}
	if state.BlockchainState.IsAbsent() || state.BlockchainState.MustGet().Peak.IsAbsent() {
		return fmt.Errorf("blockchain state or peak not present in the response")
	}

	height := state.BlockchainState.MustGet().Peak.MustGet().Height
	start := height - m.rpcPerPage
	end := height

	bar := progressbar.Default(int64(height))
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
	}

	return nil
}

func (m *Metrics) fetchAndSaveBlocksBetween(start, end uint32) error {
	blocks, _, err := m.httpClient.FullNodeService.GetBlocks(&rpc.GetBlocksOptions{
		Start: int(start),
		End:   int(end),
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
func (m *Metrics) FillBlockGaps() error {
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
	for rows.Next() {
		err = rows.Scan(&start, &end)
		if err != nil {
			return err
		}

		err = m.fetchAndSaveBlocksBetween(start, end+1) // end is not inclusive in this func, so adding 1
		if err != nil {
			return err
		}
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
		result, _, err := m.httpClient.FullNodeService.GetBlockByHeight(&rpc.GetBlockByHeightOptions{BlockHeight: int(block.Height)})
		if err != nil {
			log.Errorf("Error getting block in response to webhook: %s\n", err.Error())
			return
		}

		if result.Block.IsAbsent() {
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

func (m *Metrics) saveBlock(block types.FullBlock) error {
	blockHeight := block.RewardChainBlock.Height
	farmerPuzzHash := block.Foliage.FoliageBlockData.FarmerRewardPuzzleHash.String()
	farmerAddress, _ := bech32m.EncodePuzzleHash(block.Foliage.FoliageBlockData.FarmerRewardPuzzleHash, "xch")
	insert, err := m.mysqlClient.Query("INSERT INTO blocks (height, farmer_puzzle_hash, farmer_address) VALUES(?, ?, ?)", blockHeight, farmerPuzzHash, farmerAddress)
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
	// @TODO need a lock for actually processing this, so we only do one at a time (and if there's a queue, just let the latest one be the one waiting always)
	err := m.FillBlockGaps()
	if err != nil {
		log.Errorf("error backfilling gaps: %s\n", err.Error())
		return
	}

	nakamoto50, err := m.calculateNakamoto(peakHeight, 50)
	if err != nil {
		log.Errorf("Error calculating 50%% threshold nakamoto coefficient: %s\n", err.Error())
		return
	}

	nakamoto51, err := m.calculateNakamoto(peakHeight, 51)
	if err != nil {
		log.Errorf("Error calculating 51%% threshold nakamoto coefficient: %s\n", err.Error())
		return
	}

	m.prometheusMetrics.nakamotoCoefficient50.Set(float64(nakamoto50))
	m.prometheusMetrics.nakamotoCoefficient51.Set(float64(nakamoto51))
	m.prometheusMetrics.blockHeight.Set(float64(peakHeight))
}

func (m *Metrics) calculateNakamoto(peakHeight uint32, thresholdPercent int) (int, error) {
	query := "select number, cumulative_percent from ( " +
		"select " +
		"        row_number() over (order by count(*) desc) as number, " +
		"        farmer_address, " +
		"        count(*) as count, " +
		"        sum(count(*)) over (order by count(*) desc) as cumulative_count, " +
		"        count(*)/? as percent, " +
		"        sum(count(*)) over (order by count(*) desc) / ? as cumulative_percent " +
		"    from blocks where height > ? group by farmer_address order by count DESC limit 100 " +
		") as intermediary " +
		"where cumulative_percent > ? order by cumulative_percent asc, number asc limit 1;"
	// 1: lookbackWindowPercent
	// 2: lookbackWindowPercent
	// 3: minHeight
	// 4: thresholdPercent
	lookbackWindowPercent := m.lookbackWindow / 100
	minHeight := peakHeight - m.lookbackWindow + 1
	rows, err := m.mysqlClient.Query(query, lookbackWindowPercent, lookbackWindowPercent, minHeight, thresholdPercent)
	if err != nil {
		return 0, err
	}
	defer func(rows *sql.Rows) {
		err := rows.Close()
		if err != nil {
			log.Errorf("Could not close rows: %s\n", err.Error())
		}
	}(rows)

	var (
		number            int
		cumulativePercent float64
	)
	rows.Next()
	err = rows.Scan(&number, &cumulativePercent)
	if err != nil {
		return 0, err
	}

	return number, nil
}
