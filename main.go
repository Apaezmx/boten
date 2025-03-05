package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"boten.ai/boten/handlers"
	"cloud.google.com/go/firestore"
	"github.com/joho/godotenv"
)

func main() {
	ctx := context.Background()
	err := godotenv.Load(".env")
	if err != nil {
		log.Fatalf("Error loading .env file: %s", err)
	}
	client, err := firestore.NewClient(ctx, "retaissance")
	if err != nil {
		panic(err)
	}
	query := client.Collection("users").Where("email", "==", "paezand@gmail.com").Limit(1)
	docs, err := query.Documents(ctx).GetAll()
	if err != nil {
		panic(err)
	}
	fmt.Print(docs)
	defer client.Close()

	handler := handlers.NewRHandler(client)
	s := &http.Server{
		Addr:           ":8080",
		Handler:        handler,
		ReadTimeout:    10 * time.Second,
		WriteTimeout:   10 * time.Second,
		MaxHeaderBytes: 1 << 20,
	}
	log.Println("Retissance AI Backend Server started on port 8080")
	log.Fatal(s.ListenAndServe())
}
