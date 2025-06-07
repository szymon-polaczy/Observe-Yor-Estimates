package main

import (
	"fmt"
)

// AddSampleData adds sample task data for testing the daily update functionality
func AddSampleData() {
	db, err := GetDB()
	if err != nil {
		fmt.Printf("Error opening database: %v\n", err)
		return
	}
	defer db.Close()

	// Add sample tasks with estimations
	sampleTasks := []struct {
		taskID     int
		parentID   int
		assignedBy int
		name       string
		level      int
		rootGroup  int
	}{
		{1001, 0, 1, "Implement user authentication [8-12]", 1, 100},
		{1002, 1001, 1, "Create login form [2-4]", 2, 100},
		{1003, 1001, 1, "Setup JWT tokens [15-10]", 2, 100}, // broken estimation
		{1004, 0, 2, "Database optimization", 1, 200},       // no estimation
		{1005, 0, 1, "API development [5-8]", 1, 100},
	}

	// Insert sample tasks
	for _, task := range sampleTasks {
		_, err := db.Exec(`INSERT OR REPLACE INTO tasks VALUES (?, ?, ?, ?, ?, ?)`,
			task.taskID, task.parentID, task.assignedBy, task.name, task.level, task.rootGroup)
		if err != nil {
			fmt.Printf("Error inserting task %d: %v\n", task.taskID, err)
			continue
		}

		// Add some history entries to simulate changes
		err = TrackTaskChange(db, task.taskID, task.name, "created", "", task.name)
		if err != nil {
			fmt.Printf("Error tracking task change for %d: %v\n", task.taskID, err)
		}
	}

	fmt.Println("Sample data added successfully!")
}
