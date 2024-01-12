# Block Metrics

This repo connects to a chia full node and maintains a MySQL database with some metadata about blocks which can then be
used to query and generate block metrics. Additionally, the metrics are exported via a prometheus compatible `/metrics`
endpoint.

## Exported Metrics

### Nakamoto Coefficient > 50%
Prometheus Name: `chia_block_metrics_nakamoto_coefficient_gt50`

Nakamoto coefficient (number of nodes required to collude for a majority) calculated at >50% of nodes.

### Nakamoto Coefficient > 51%

Nakamoto coefficient (number of nodes required to collude for a majority) calculated at >51% of nodes.

Prometheus Name: `chia_block_metrics_nakamoto_coefficient_gt51`

### Nakamoto Coefficient Adjusted > 50%

Adjusted nakamoto coefficient (number of nodes required to collude for a majority) calculated at >50% of nodes. The adjusted figure ignores certain farmer addresses (configurable with `adjusted-ignore-addresses`) in the calculation. This adjustment capability allows for accurate metrics with alternate farmer/harvester implementations that redirect a portion of farmer rewards to the developer's address. The default NC includes these dev addresses as large farmers, but since the individual farmers sign their own blocks, the Adjusted NC removes these dev addresses from the calculations.

Prometheus Name: `chia_block_metrics_nakamoto_coefficient_gt50_adjusted`

### Nakamoto Coefficient Adjusted > 51%

Adjusted nakamoto coefficient (number of nodes required to collude for a majority) calculated at >51% of nodes. The adjusted figure ignores certain farmer addresses (configurable with `adjusted-ignore-addresses`) in the calculation. This adjustment capability allows for accurate metrics with alternate farmer/harvester implementations that redirect a portion of farmer rewards to the developer's address. The default NC includes these dev addresses as large farmers, but since the individual farmers sign their own blocks, the Adjusted NC removes these dev addresses from the calculations.

Prometheus Name: `chia_block_metrics_nakamoto_coefficient_gt51_adjusted`

### Block Height

The peak block height in the database, which the metrics are calculated based on.

Prometheus Name: `chia_block_metrics_nakamoto_coefficient_gt51_adjusted`

## Database Structure

The database current has a single `blocks` table with the following fields:

| Column             | Description                                                                                                                                           |
|--------------------|-------------------------------------------------------------------------------------------------------------------------------------------------------|
| timestamp          | Timestamp of this block. Only TX blocks have timestamps on chain. In this DB, other blocks use the next transaction block's timestamp for this field. |
| height             | the height of this block                                                                                                                              |
| transaction_block  | Whether or not this block is a transaction block                                                                                                      |
| farmer_puzzle_hash | The puzzle hash the farmer reward was sent to for this block                                                                                          |
| farmer_address     | The address the farmer reward was sent to for this block                                                                                              |

## Installation / Usage


