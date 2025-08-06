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

	//"text/template/parse"

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

	const maxMemory = 10 << 20                   // 10 MB is 10 * 1024 * 1024, which is 20 bitshifts
	r.ParseMultipartForm(maxMemory)              // Parse the form data with a maximum memory limit, overflow will be stored on disk
	file, header, err := r.FormFile("thumbnail") // read the while thumbnail file from the form data
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Couldn't get thumbnail file", err)
		return
	}
	defer file.Close()

	if header.Size > maxMemory { // File size is sent with header.Size, so we don't need to read the file to check its size
		respondWithError(w, http.StatusBadRequest, "Thumbnail file too large", nil)
		return
	}

	content := header.Header.Get("Content-Type")      // There are three parts to *multipart.Header: Header (map), Size (int64), and Filename("string")
	mediaType, _, err := mime.ParseMediaType(content) // Parse the media type to get the content type, e.g. "image/jpeg", and any parameters
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Couldn't parse thumbnail file type", err)
		return
	}

	if mediaType != "image/jpeg" && mediaType != "image/png" {
		respondWithError(w, http.StatusBadRequest, "Invalid thumbnail file type", nil)
		return
	}
	// Check if the video exists and if the user is allowed to upload a thumbnail for it
	video, err := cfg.db.GetVideo(videoID)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't get video metadata", err)
		return
	}

	if video.UserID != userID { // Make sure the requester is the owner of the video
		respondWithError(w, http.StatusUnauthorized, "You are not allowed to upload a thumbnail for this video", nil)
		return
	}
	//Create new randome filename when uploading a thumbnail
	newFilenameBytes := make([]byte, 32) // Generate a new random filename for the thumbnail, 32 bytes is 256 bits
	_, err = rand.Read(newFilenameBytes) // Fill the byte slice with random data
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't generate random filename", err)
		return
	}
	randvideo := base64.RawURLEncoding.EncodeToString(newFilenameBytes) // Generate a new filename for the thumbnail, e.g. "videoID.jpg"
	//End random filename generation
	requestPath := strings.Split(mediaType, "/")
	fileExtension := requestPath[len(requestPath)-1] // Get the file extension from the media type, e.g. "image/jpeg" -> "jpeg"
	filename := fmt.Sprintf("%s.%s", randvideo, fileExtension)
	filepath := filepath.Join(cfg.assetsRoot, filename)
	emptyFile, err := os.Create(filepath) // Create a new empty file to copy the info into
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't create thumbnail file", err)
		return
	}
	defer emptyFile.Close()
	_, err = io.Copy(emptyFile, file) // Write the file data to the new file
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't write thumbnail file", err)
		return
	}

	port := cfg.port
	thumbnail := fmt.Sprintf("http://localhost:%s/assets/%s", port, filename)

	ptrUrl := &thumbnail

	video.ThumbnailURL = ptrUrl
	err = cfg.db.UpdateVideo(video)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't update video metadata", err)
		return
	}

	video, err = cfg.db.GetVideo(videoID) // Get the updated video metadata
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't get video metadata", err)
		return
	}

	respondWithJSON(w, http.StatusOK, video)
}
