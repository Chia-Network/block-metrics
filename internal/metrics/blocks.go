package metrics

import (
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
		if start < 0 {
			start = 0
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
			err = m.saveBlock(block)
			if err != nil {
				return err
			}
		}

		err = bar.Add(int(m.rpcPerPage))
		_ = err // Just the progress bar, so it's not critical

		if start <= 0 {
			err = bar.Finish()
			_ = err // Just the progress bar, so it's not critical
			break
		}
		start = start - m.rpcPerPage
		end = end - m.rpcPerPage
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
