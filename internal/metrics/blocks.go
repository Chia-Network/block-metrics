package metrics

import (
	"fmt"
	"log"

	"github.com/chia-network/go-chia-libs/pkg/bech32m"
	"github.com/chia-network/go-chia-libs/pkg/rpc"
	"github.com/schollz/progressbar/v3"
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
	start := uint32(0)
	end := m.rpcPerPage

	bar := progressbar.Default(int64(height))
	for {
		if end > height {
			end = height
		}
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
		}

		err = bar.Add(int(m.rpcPerPage))
		_ = err // Just the progress bar, so it's not critical

		if end >= height {
			err = bar.Finish()
			_ = err // Just the progress bar, so it's not critical
			break
		}
		start = start + m.rpcPerPage
		end = end + m.rpcPerPage
	}

	return nil
}
