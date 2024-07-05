package main

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	_ "github.com/lib/pq"
	"github.com/rwcarlsen/goexif/exif"
	_ "image/jpeg"
	"log"
	"os"
)

func HandleRequest(ctx context.Context, event events.S3Event) (*string, error) {
	connStr := os.Getenv("DB_URL")
	db, err := sql.Open("postgres", connStr)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %s", err)
	}
	defer db.Close()

	err = db.Ping()
	if err != nil {
		return nil, fmt.Errorf("failed to ping database: %s", err)
	}
	fmt.Println("Successfully connected to the database!")

	sdkConfig, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to load SDK config: %s", err)
	}
	s3Client := s3.NewFromConfig(sdkConfig)

	var bucket string
	var key string
	for _, record := range event.Records {
		bucket = record.S3.Bucket.Name
		key = record.S3.Object.URLDecodedKey

		// Get the object
		getObjectOutput, err := s3Client.GetObject(ctx, &s3.GetObjectInput{
			Bucket: &bucket,
			Key:    &key,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to get object %s/%s: %s", bucket, key, err)
		}
		defer getObjectOutput.Body.Close()

		// Read the object data into a buffer
		buf := new(bytes.Buffer)
		_, err = buf.ReadFrom(getObjectOutput.Body)
		if err != nil {
			return nil, fmt.Errorf("failed to read object %s/%s: %s", bucket, key, err)
		}

		// Check EXIF data
		exifData, err := exif.Decode(buf)
		if err != nil {
			return nil, fmt.Errorf("failed to decode EXIF data: %s", err)
		}

		log.Printf("successfully retrieved %s/%s with EXIF DateTime: %v", bucket, key, exifData)

		// SQL statement
		sqlStatement := `INSERT INTO images (bucket, key, model, company) VALUES ($1,$2,$3,$4)`

		// Execute the insertion
		model, err := exifData.Get(exif.Model)
		if err != nil {
			return nil, fmt.Errorf("failed to get model: %s", err)
		}
		company, err := exifData.Get(exif.Make)
		if err != nil {
			return nil, fmt.Errorf("failed to get company: %s", err)
		}
		_, err = db.Exec(sqlStatement, bucket, key, model.String(), company.String())
		if err != nil {
			return nil, fmt.Errorf("failed to execute SQL statement: %s", err)
		}
	}

	return nil, nil
}

func main() {
	lambda.Start(HandleRequest)
}
