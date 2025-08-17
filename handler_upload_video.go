package main

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"strings"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/auth"
	"github.com/google/uuid"
)

func (cfg *apiConfig) handlerUploadVideo(w http.ResponseWriter, r *http.Request) {
	maxUploadSize := int64(1 << 30) // 1GB

	r.Body = http.MaxBytesReader(w, r.Body, maxUploadSize) // MaxBytesReader wraps request body to limit its size

	videoIDString := r.PathValue("videoID")
	videoID, err := uuid.Parse(videoIDString) // Turn the videoID string into a UUID
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid video ID", err)
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

	video, err := cfg.db.GetVideo(videoID) // Get the video metadata from the database, which was created earlier
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't get video metadata", err)
		return
	}

	if video.UserID != userID { // Make sure the requester is the owner of the video
		respondWithError(w, http.StatusUnauthorized, "You are not the owner of this video", nil)
		return
	}

	r.ParseMultipartForm(maxUploadSize)      // Parse the form data with a maximum memory limit, overflow will be stored on disk
	file, header, err := r.FormFile("video") // read the while video file from the form data
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Couldn't get file", err)
		return
	}
	defer file.Close() ///super important, don't forget this

	if header.Size > maxUploadSize { // Redundant, but may provide extra security. File size is sent with header.Size, so we don't need to read the file to check its size
		respondWithError(w, http.StatusBadRequest, "Video file too large", nil)
		return
	}

	content := header.Header.Get("Content-Type")      // There are three parts to *multipart.Header: Header (map), Size (int64), and Filename("string")
	mediaType, _, err := mime.ParseMediaType(content) // Parse the media type to get the content type, e.g. "image/jpeg", and any parameters
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Couldn't parse thumbnail file type", err)
		return
	}

	if mediaType != "video/mp4" {
		respondWithError(w, http.StatusBadRequest, "Invalid video file type", nil)
		return
	}
	tempVideo, err := os.CreateTemp("", "tubely-upload.mp4") //Make a new temp file in default directory with prefix tubely-upload and suffix .mp4
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't create temp file", err)
		return
	}
	defer os.Remove(tempVideo.Name())       // Clean up the temp file afterwards
	defer tempVideo.Close()                 // defer is LIFO, so this will run before the os.Remove
	copied, err := io.Copy(tempVideo, file) // Copy the file contents to the temp file
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't copy video file", err)
		return
	}

	if copied != header.Size { // Make sure the copied size matches the header size
		respondWithError(w, http.StatusInternalServerError, "Copied video file size does not match header size", nil)
		return
	}

	_, err = tempVideo.Seek(0, io.SeekStart) // Reset the file pointer to the beginning of the file
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't seek to beginning of temp file", err)
		return
	}
	aspectRatio, err := getVideoAspectRatio(tempVideo.Name()) // Get the aspect ratio of the video file
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't get video aspect ratio", err)
		return
	}
	// Set the aspect ratio in the video metadata
	var aspectPrefix string
	if aspectRatio == "16:9" {
		aspectPrefix = "landscape"
	} else if aspectRatio == "9:16" {
		aspectPrefix = "portrait"
	} else {
		aspectPrefix = "other"
	}
	//Create new randome filename when uploading a thumbnail
	newFilenameBytes := make([]byte, 32) // Generate a new random filename for the video, 32 bytes is 256 bits
	_, err = rand.Read(newFilenameBytes) // Fill the byte slice with random data
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't generate random filename", err)
		return
	}
	randvideo := base64.RawURLEncoding.EncodeToString(newFilenameBytes) // Generate a new filename for the thumbnail, e.g. "videoID.jpg"
	//End random filename generation
	requestPath := strings.Split(mediaType, "/")
	fileExtension := requestPath[len(requestPath)-1] // Get the file extension from the media type, e.g. "image/jpeg" -> "jpeg"
	filename := fmt.Sprintf("%s/%s.%s", aspectPrefix, randvideo, fileExtension)
	s3Info := &s3.PutObjectInput{
		Bucket:      &cfg.s3Bucket,
		Key:         &filename,
		Body:        tempVideo,
		ContentType: &content,
	}

	_, err = cfg.s3Client.PutObject(r.Context(), s3Info) // Upload the file to S3
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't upload video file to S3", err)
		return
	}
	//url := fmt.Sprintf("https://%s.s3.%s.amazonaws.com/%s", cfg.s3Bucket, cfg.s3Region, filename)
	url := cfg.getObjectURL(filename) //fmt.Sprintf("https://%s.s3.%s.amazonaws.com/%s", cfg.s3Bucket, cfg.s3Region, filename)
	video.VideoURL = &url
	err = cfg.db.UpdateVideo(video)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't update video metadata", err)
		return
	}
	respondWithJSON(w, http.StatusOK, video)
}
