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

`make build` will build the app and put the resulting binary in `bin/block-metrics`. The app needs a MySQL database to
store the block information within.

### Configuration Flags

The following configuration flags are available. These can be passed as flags at runtime, set in a yml file at
`~/.block-metrics.yaml`, or set as env vars prefixed with `BLOCK_METRICS_`, converting `-` to `_`, and all upper case.

`adjusted-ignore-addresses` is a list of addresses to ignore in the adjusted NC metric

`chia-hostname` The hostname to use to connect to the full node (default `localhost`)

`db-host` The hostname or IP address for the mysql server

`db-name` The name of the database in MySQL to store the block metrics data in

`db-password` The password for the database

`db-port` The port for the database (default 3306)

`db-user` The username to use when connecting to the DB

`lookback-window` How many blocks to look at when calculating the nakamoto coefficient (Default 32256)

`metrics-port` The port to run the prometheus metrics server on

`rpc-per-page` How many results to fetch in each RPC call when backfilling block information

### Commands

#### Serve

`block-metrics serve`

The primary way to run the app is the `serve` command. This connects to the chia full node and listen for new blocks
and adds them to the database. Each time a block is finished processing, the metrics are recalculated. 

#### Backfill Blocks

`block-metrics backfill-blocks [--delete-first]`

This command backfills missing data from the full node into the database. If the `--delete-first` flag is used, the
contents in the table will be deleted before reimporting.

#### Historical Output

`block-metrics historical-output [--interval 100]`

Generates a `history.csv` file with historical nakamoto coefficient data every <interval> blocks, based on the data
present in the database. To export a full history of the chain, you must first backfill all missing blocks. 
