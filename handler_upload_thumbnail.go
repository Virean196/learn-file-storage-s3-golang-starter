package main

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"

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

	video_md, err := cfg.db.GetVideo(videoID)
	if video_md.UserID != userID {
		respondWithError(w, http.StatusUnauthorized, "unauthorized access", err)
		return
	}
	mediaType, _, err := mime.ParseMediaType(header.Header.Get("Content-Type"))

	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "error parsing media type", err)
		return
	}

	if mediaType != "image/jpeg" && mediaType != "image/png" {
		respondWithError(w, http.StatusNotAcceptable, "invalid file type for thumbail, please use image/jpeg or image/png", err)
		return
	}

	randKey := make([]byte, 32)
	rand.Read(randKey)
	fileName := fmt.Sprintf("%s.%s", base64.RawURLEncoding.EncodeToString(randKey), strings.Split(mediaType, "/")[1])
	filePath := filepath.Join(cfg.assetsRoot, fileName)
	thumbailFile, err := os.Create(filePath)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "error creating file", err)
		return
	}
	defer thumbailFile.Close()

	_, err = io.Copy(thumbailFile, file)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "error copying file", err)
		return
	}
	defer file.Close()

	port := os.Getenv("PORT")
	if port == "" {
		port = "8091"
	}

	newURL := fmt.Sprintf("http://localhost:%s/assets/%s", port, fileName)
	video_md.ThumbnailURL = &newURL

	err = cfg.db.UpdateVideo(video_md)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "error updating video", err)
		return
	}
	respondWithJSON(w, http.StatusOK, video_md)
}
