package main

import (
	"fmt"
	"io"
	"net/http"
	"os"

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

	const maxMemory = 10 << 20
	err = r.ParseMultipartForm(maxMemory)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "malformed form", err)
		return
	}
	file, header, err := r.FormFile("thumbnail")
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "error forming file", err)
		return
	}
	data, err := io.ReadAll(file)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "error reading file", err)
		return
	}

	mediaType := header.Header.Get("Content-Type")
	video_md, err := cfg.db.GetVideo(videoID)
	if video_md.UserID != userID {
		respondWithError(w, http.StatusUnauthorized, "unauthorized access", err)
		return
	}

	tn := thumbnail{
		mediaType: mediaType,
		data:      data,
	}

	videoThumbnails[videoID] = tn

	port := os.Getenv("PORT")
	if port == "" {
		port = "8091"
	}
	new_tnURL := fmt.Sprintf("http://localhost:%s/api/thumbnails/%s", port, videoID)
	video_md.ThumbnailURL = &new_tnURL

	err = cfg.db.UpdateVideo(video_md)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "error updating video", err)
		return
	}
	respondWithJSON(w, http.StatusOK, video_md)
	defer r.Body.Close()
}
