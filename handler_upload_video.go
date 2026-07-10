package main

import (
	"net/http"
	"mime"
	"fmt"
	"os"
	"io"
	"bytes"
	"os/exec"
	"github.com/google/uuid"
	"encoding/base64"
	"crypto/rand"
	"encoding/json"
	"errors"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/auth"
)


type FFProbeOutput struct {
	Streams []Stream `json:"streams"`
}

type Stream struct {
	CodecType string `json:"codec_type"`
	Width     int    `json:"width"`
	Height    int    `json:"height"`
}

func (o FFProbeOutput) VideoStream() *Stream {
	for i := range o.Streams {
		if o.Streams[i].CodecType == "video" {
			return &o.Streams[i]
		}
	}
	return nil
}

func getVideoAspectRatio(filePath string) (string, error) {
	cmd := exec.Command(
		"ffprobe",
		"-v", "error",
		"-print_format", "json",
		"-show_streams",
		filePath,
	)

	var out bytes.Buffer
	cmd.Stdout = &out

	if err := cmd.Run(); err != nil {
		return "", err
	}

	var output FFProbeOutput
	if err := json.Unmarshal(out.Bytes(), &output); err != nil {
		return "", err
	}

	video := output.VideoStream()
	if video == nil {
		return "", errors.New("no video stream found")
	}

	switch {
	case video.Width > video.Height:
		return "landscape", nil
	case video.Width < video.Height:
		return "portrait", nil
	default:
		return "square", nil
	}
}

func processVideoForFastStart(filePath string) (string, error) {

	outputFile := fmt.Sprintf("%s.processing", filePath)

	cmd := exec.Command(
		"ffmpeg",
		"-i", filePath,
		"-c", "copy",
		"-movflags", "faststart",
		"-f", "mp4",
		outputFile,
	)

	if err := cmd.Run(); err != nil {
		return "", err
	}

	return outputFile, nil

}

func (cfg *apiConfig) handlerUploadVideo(w http.ResponseWriter, r *http.Request) {

	videoIDString := r.PathValue("videoID")
	videoID, err := uuid.Parse(videoIDString)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid ID", err)
		return
	}

	token, err := auth.GetBearerToken(r.Header)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Couldn't find JWT", err)
		return
	}

	userID, err := auth.ValidateJWT(token, cfg.jwtSecret)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Couldn't validate JWT", err)
		return
	}

	video, err := cfg.db.GetVideo(videoID)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't find video", err)
		return
	}
	if video.UserID != userID {
		respondWithError(w, http.StatusUnauthorized, "Not authorized to update this video", nil)
		return
	}

	const maxUploadSize = 1 << 30 // 1 GiB
	const maxMemory = 10 << 20    // 10 MiB

	r.Body = http.MaxBytesReader(w, r.Body, maxUploadSize)
	r.ParseMultipartForm(maxMemory)

	file, header, err := r.FormFile("video")
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Unable to parse form file", err)
		return
	}
	defer file.Close()

	mediaType, _, err := mime.ParseMediaType(header.Header.Get("Content-Type"))
	if err != nil {
    	respondWithError(w, http.StatusBadRequest, "Invalid Content-Type", err)
    	return
	}
	if mediaType != "video/mp4" {
    	respondWithError(w, http.StatusBadRequest, "Content-Type must be video/mp4", nil)
    	return
	}

	f, err := os.CreateTemp("", "tubely-upload.mp4")
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't create temp file", err)
		return
	}
	defer os.Remove(f.Name())
	defer f.Close()

	_, err = io.Copy(f, file)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't save temp file", err)
		return
	}

	_, err = f.Seek(0, io.SeekStart)
	if err != nil {
    	return 
	}

	processedPath, err := processVideoForFastStart(f.Name())
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't process video", err)
		return
	}
	defer os.Remove(processedPath)

	aspectRatio, err := getVideoAspectRatio(f.Name())
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't save temp file", err)
		return
	}

	processedFile, err := os.Open(processedPath)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't open processed video", err)
		return
	}
	defer processedFile.Close()

	fmt.Println(aspectRatio)

	// Generate a unique object key
	bytes := make([]byte, 32)
	rand.Read(bytes)
	name := base64.RawURLEncoding.EncodeToString(bytes)
	key := fmt.Sprintf("%s/%s.mp4", aspectRatio, name,)
	
	// Upload to S3
	_, err = cfg.s3Client.PutObject(r.Context(), &s3.PutObjectInput{
		Bucket:      aws.String(cfg.s3Bucket),
		Key:         aws.String(key),
		Body:        processedFile,
		ContentType: aws.String(mediaType),
	})
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't upload video", err)
		return
	}

	// Update the video's URL
	location := fmt.Sprintf("%s%s", cfg.s3CfDistribution, key)
	fmt.Println(location)
	video.VideoURL = &location
	err = cfg.db.UpdateVideo(video)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't update video", err)
		return
	}

	respondWithJSON(w, http.StatusOK, video)
}
