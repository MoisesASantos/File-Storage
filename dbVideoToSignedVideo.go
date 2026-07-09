package main

import (
	"strings"
	"context"
	"time"
	"fmt"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/database"

)


func generatePresignedURL(s3Client *s3.Client, bucket, key string, expireTime time.Duration,) (string, error) {
	
	presignClient := s3.NewPresignClient(s3Client)

	req, err := presignClient.PresignGetObject(
		context.Background(),
		&s3.GetObjectInput{
			Bucket: aws.String(bucket),
			Key:    aws.String(key),
		},
		s3.WithPresignExpires(expireTime),
	)
	if err != nil {
		return "", err
	}

	return req.URL, nil
}

func (cfg *apiConfig) DbVideoToSignedVideo(video database.Video) (database.Video, error) {
	
	if video.VideoURL == nil {
		return video, nil
	}

	fmt.Println("stored video url:", *video.VideoURL)

	parts := strings.Split(*video.VideoURL, ",")
	fmt.Println(parts)
	if len(parts) != 2 {
		return database.Video{}, fmt.Errorf("invalid video url")
	}

	url, err := generatePresignedURL(cfg.s3Client, parts[0], parts[1], 5*time.Minute,)
	if err != nil {
		return database.Video{}, err
	}

	video.VideoURL = &url

	return video, nil
}
