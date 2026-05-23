package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"mime"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/auth"
	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/database"
	"github.com/google/uuid"
)

func (cfg *apiConfig) handlerUploadVideo(w http.ResponseWriter, r *http.Request) {

	r.Body = http.MaxBytesReader(w, r.Body, 1<<30)
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

	fmt.Println("uploading video", videoID, "by user", userID)

	video, err := cfg.db.GetVideo(videoID)
	if err != nil {
		respondWithError(w, http.StatusNotFound, "Unable to find video", err)
		return
	}
	if video.UserID != userID {
		respondWithError(w, http.StatusUnauthorized, "Invalid user", err)
		return
	}

	videoFile, headers, err := r.FormFile("video")
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Unable to form video", err)
		return
	}

	defer videoFile.Close()

	mediaType, _, err := mime.ParseMediaType(headers.Header.Get("Content-Type"))
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Unable to parse media type", err)
		return
	}
	if mediaType != "video/mp4" {
		respondWithError(w, http.StatusBadRequest, "Invalid video format, use mp4", err)
		return
	}
	tempFile, err := os.CreateTemp("", "tubely-upload.mp4")
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Unable to create temp video file", err)
		return
	}
	defer os.Remove(tempFile.Name())
	defer tempFile.Close()

	io.Copy(tempFile, videoFile)

	processedVideoPath, err := processVideoForFastStart(tempFile.Name())
	if err != nil {
		respondWithError(w, http.StatusNotAcceptable, "Video could not be processed", err)
		return
	}
	processedVideo, err := os.Open(processedVideoPath)
	if err != nil {
		respondWithError(w, http.StatusNotAcceptable, "Unable to open processed video", err)
		return
	}
	aspectRatio, err := getVideoAspectRatio(tempFile.Name())
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Unable to get aspect ration for video", err)
		return
	}
	var prefix string
	switch aspectRatio {
	case "16:9":
		prefix = "landscape"
	case "9:16":
		prefix = "portrait"
	default:
		prefix = "other"
	}

	tempFile.Seek(0, io.SeekStart)
	key := make([]byte, 32)
	rand.Read(key)
	hexKey := fmt.Sprintf("%s/%s.%s", prefix, hex.EncodeToString(key), strings.Split(mediaType, "/")[1])
	cfg.s3Client.PutObject(context.Background(), &s3.PutObjectInput{Bucket: &cfg.s3Bucket, Key: &hexKey, Body: processedVideo, ContentType: &mediaType})
	/* newVidUrl := fmt.Sprintf("https://%s.s3.%s.amazonaws.com/%s", cfg.s3Bucket, cfg.s3Region, hexKey) */
	newVidUrl := []string{cfg.s3Bucket, hexKey}
	result := strings.Join(newVidUrl, ",")
	video.VideoURL = &result
	new_vid, err := cfg.dbVideoToSignedVideo(video)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Unable to create presigned url", err)
		return
	}
	err = cfg.db.UpdateVideo(new_vid)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Failed to update video", err)
		return
	}

}

func generatePresignedURL(s3client *s3.Client, bucket *string, key string, expireTime time.Duration) (string, error) {
	s3PresignClient := s3.NewPresignClient(s3client)
	presignedRequest, err := s3PresignClient.PresignGetObject(context.Background(), &s3.GetObjectInput{Bucket: bucket, Key: &key}, s3.WithPresignExpires(expireTime))
	if err != nil {
		return "", err
	}
	return presignedRequest.URL, nil
}

func (cfg *apiConfig) dbVideoToSignedVideo(video database.Video) (database.Video, error) {
	videoUrlList := strings.Split(*video.VideoURL, ",")
	presignedUrl, err := generatePresignedURL(cfg.s3Client, &videoUrlList[0], videoUrlList[1], 5*time.Minute)
	if err != nil {
		log.Print("Unable to generate presigned url")
		return database.Video{}, err
	}
	video.VideoURL = &presignedUrl
	return video, nil
}
