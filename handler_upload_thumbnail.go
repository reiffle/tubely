package main

import (
	"encoding/base64"
	"fmt"
	"io"
	"net/http"

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

	mediaType := header.Header.Get("Content-Type") // There are three parts to *multipart.Header: Header (map), Size (int64), and Filename("string")
	if mediaType != "image/jpeg" && mediaType != "image/png" {
		respondWithError(w, http.StatusBadRequest, "Invalid thumbnail file type", nil)
		return
	}

	fileData, err := io.ReadAll(file) // Be sure to read ALL file date into the variable, hence io.ReadAll
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't read thumbnail file", err)
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

	data := base64.StdEncoding.EncodeToString(fileData)            // Encode the thumbnail data to base64 string
	thumbnail := fmt.Sprintf("data:%s;base64,%s", mediaType, data) // Create a data URL for the thumbnail
	//Could also use "github.com/vincent-petithory/dataurl" dataURL := dataurl.New(data, mimeType)

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
