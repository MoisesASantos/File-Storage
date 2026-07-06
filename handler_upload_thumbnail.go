package main

import (
	"fmt"
	"net/http"
	"io"
	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/auth"
	"github.com/google/uuid"
)

func (cfg *apiConfig) handlerUploadThumbnail(w http.ResponseWriter, r *http.Request) {
	
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


	fmt.Println("uploading thumbnail for video", videoID, "by user", userID)

	// TODO: implement the upload here
	const maxMemory = 10 << 20
	r.ParseMultipartForm(maxMemory)

	file, _, err := r.FormFile("thumbnail")
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Unable to parse form file", err)
		return
	}
	defer file.Close()

	contentType := r.Header.Get("Content-Type")
	if contentType == "" {
		respondWithError(w, http.StatusBadRequest, "Unable to parse form file", err)
		return
	}

	data, err := io.ReadAll(file)
	if err != nil {
		return
	}

	video, err := cfg.db.GetVideo(videoID)
	if err != nil {

	}

	params := thumbnail{
		data:	data,
		mediaType: contentType,
	}
	
	videoThumbnails[video.ID] = params
	url := fmt.Sprintf("http://localhost:8091/api/thumbnails/%v", video.ID,)
	video.ThumbnailURL = &url

	err = cfg.db.UpdateVideo(video)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Unable to update the video", err)
		return
	}
	fmt.Println(video)

	respondWithJSON(w, http.StatusOK, video)
}
