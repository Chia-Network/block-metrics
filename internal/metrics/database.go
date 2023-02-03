package metrics

// initTables ensures that the tables required exist and have the correct columns present
func (m *Metrics) initTables() error {
	query := "CREATE TABLE IF NOT EXISTS `blocks` (" +
		"  `id` int unsigned NOT NULL AUTO_INCREMENT," +
		"  `timestamp` DATETIME DEFAULT NULL," +
		"  `height` int DEFAULT NULL," +
		"  `farmer_puzzle_hash` varchar(255) DEFAULT NULL," +
		"  `farmer_address` varchar(255) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci DEFAULT NULL," +
		"  PRIMARY KEY (`id`)" +
		") ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;"

	result, err := m.mysqlClient.Query(query)
	if err != nil {
		return err
	}
	return result.Close()
}

// DeleteBlockRecords deletes all records from the blocks table in the database
func (m *Metrics) DeleteBlockRecords() error {
	query := "DELETE from blocks;"
	result, err := m.mysqlClient.Query(query)
	if err != nil {
		return err
	}
	return result.Close()
}
