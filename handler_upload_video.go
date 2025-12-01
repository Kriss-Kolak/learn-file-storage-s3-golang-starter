package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/auth"
	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/database"
	"github.com/google/uuid"
)

func (cfg *apiConfig) handlerUploadVideo(w http.ResponseWriter, r *http.Request) {

	maxMemory := 1 << 30
	r.Body = http.MaxBytesReader(w, r.Body, int64(maxMemory))
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

	metadata, err := cfg.db.GetVideo(videoID)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't parse media type", err)
		return
	}

	if userID != metadata.UserID {
		respondWithError(w, http.StatusUnauthorized, "Unauthorized", nil)
		return
	}

	file, fileHeader, err := r.FormFile("video")
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Couldn't form file", err)
		return
	}
	defer file.Close()
	mediaType := fileHeader.Header.Get("Content-Type")
	parsedType, _, err := mime.ParseMediaType(mediaType)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't parse media type", err)
		return
	}

	if parsedType != "video/mp4" {
		respondWithError(w, http.StatusBadRequest, "Media type is not correct", nil)
		return
	}

	tempFile, err := os.CreateTemp("", "tubely-upload.*.mp4")
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't create temp file", err)
		return
	}
	defer os.Remove(tempFile.Name())
	defer tempFile.Close()

	_, err = io.Copy(tempFile, file)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't copy data", err)
		return
	}

	tempFile.Seek(0, io.SeekStart)

	randomBytes := make([]byte, 32)

	_, err = rand.Read(randomBytes)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't create new bytes", err)
		return
	}
	aspectRatio, err := getVideoAspectRatio(tempFile.Name())
	if err != nil {
		aspectRatio = "other"
	}

	var aspectType string
	switch aspectRatio {
	case "16:9":
		aspectType = "landscape"
	case "9:16":
		aspectType = "portrait"
	default:
		aspectType = "other"
	}

	fastStartVideoPath, err := processVideoForFastStart(tempFile.Name())
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't process video for fast start", err)
		return
	}

	fastStartFile, err := os.Open(fastStartVideoPath)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't process video for fast start", err)
		return
	}

	ext := strings.ToLower(filepath.Ext(fileHeader.Filename))
	fileName := hex.EncodeToString(randomBytes)
	fullFileName := fmt.Sprintf("%s/%s%s", aspectType, fileName, ext)

	_, err = cfg.s3Client.PutObject(r.Context(), &s3.PutObjectInput{
		Bucket:      &cfg.s3Bucket,
		Key:         &fullFileName,
		Body:        fastStartFile,
		ContentType: &parsedType,
	})
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't upload file to cloud", err)
		return
	}

	//s3FileUrl := fmt.Sprintf("https://%s.s3.%s.amazonaws.com/%s", cfg.s3Bucket, cfg.s3Region, fullFileName)
	s3FileUrl := fmt.Sprintf("%s,%s", cfg.s3Bucket, fullFileName)
	metadata.VideoURL = &s3FileUrl

	err = cfg.db.UpdateVideo(metadata)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't update database", err)
		return
	}
	metadata, err = cfg.dbVideoToSignedVideo(metadata)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't get signed video", err)
	}
	respondWithJSON(w, http.StatusOK, metadata)
}

func generatePresignedURL(s3Client *s3.Client, bucket, key string, expireTime time.Duration) (string, error) {

	presignClient := s3.NewPresignClient(s3Client)
	presignedObject, err := presignClient.PresignGetObject(context.Background(), &s3.GetObjectInput{Bucket: &bucket, Key: &key}, s3.WithPresignExpires(expireTime))
	if err != nil {
		return "", err
	}
	return presignedObject.URL, nil
}

func (cfg *apiConfig) dbVideoToSignedVideo(video database.Video) (database.Video, error) {

	if video.VideoURL == nil {
		return video, nil
	}

	splited := strings.Split(*video.VideoURL, ",")

	if len(splited) != 2 {
		return video, nil
	}

	bucket := splited[0]
	key := splited[1]

	presignedURL, err := generatePresignedURL(cfg.s3Client, bucket, key, time.Duration(1*time.Minute))
	if err != nil {
		return video, err
	}
	video.VideoURL = &presignedURL
	return video, nil
}
