package models

import "time"

type JSONErrorResponse struct {
	Code    int      `json:"code"`
	Message string   `json:"message"`
	Errors  []string `json:"errors"`
}

type Cat struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
	Age  int    `json:"age"`
}

func GetCats() ([]Cat, error) {
	// Simulate Waiting time
	time.Sleep(5 * time.Second)

	return []Cat{
		{ID: 1, Name: "bri", Age: 10},
		{ID: 2, Name: "mushi", Age: 8},
	}, nil
}
