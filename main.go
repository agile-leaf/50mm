package main

import "fmt"

func main() {
	app := NewApp()

	for _, v := range app.sites {
		img := v.GetAllImageUrls()
		for _, v := range img {
			fmt.Println(v)
		}
	}
}
