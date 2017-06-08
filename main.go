package main

import (
	"fmt"
	"time"
)

func main() {
	app := NewApp()

	for  {
		for _, v := range app.sites {
			_ = v.GetAllImageUrls()
		}

		fmt.Println("Sleeping")

		time.Sleep(time.Duration(20) * time.Second)
	}
}
