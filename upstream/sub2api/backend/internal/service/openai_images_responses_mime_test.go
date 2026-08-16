package service

import (
	"encoding/base64"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOpenAIResponsesImageUploadToDataURLNormalizesFallbackMIME(t *testing.T) {
	pngBytes, err := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=")
	require.NoError(t, err)

	tests := []struct {
		name        string
		contentType string
		data        []byte
		wantPrefix  string
		wantErr     bool
	}{
		{name: "empty MIME detects PNG", data: pngBytes, wantPrefix: "data:image/png;base64,"},
		{name: "octet stream detects PNG", contentType: "application/octet-stream", data: pngBytes, wantPrefix: "data:image/png;base64,"},
		{name: "normalized octet stream detects PNG", contentType: " Application/Octet-Stream; charset=binary ", data: pngBytes, wantPrefix: "data:image/png;base64,"},
		{name: "empty MIME rejects text", data: []byte("plain text"), wantErr: true},
		{name: "octet stream rejects text", contentType: "application/octet-stream", data: []byte("plain text"), wantErr: true},
		{name: "explicit image MIME is preserved", contentType: " IMAGE/PNG; x-existing=1 ", data: []byte("not-sniffable"), wantPrefix: "data:IMAGE/PNG; x-existing=1;base64,"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dataURL, err := openAIResponsesImageUploadToDataURL(OpenAIImagesUpload{
				FileName:    "input.png",
				ContentType: tt.contentType,
				Data:        tt.data,
			})
			if tt.wantErr {
				require.Error(t, err)
				require.Empty(t, dataURL)
				return
			}
			require.NoError(t, err)
			require.True(t, strings.HasPrefix(dataURL, tt.wantPrefix), "data URL %q does not have prefix %q", dataURL, tt.wantPrefix)
		})
	}
}

func TestBuildOpenAIImagesResponsesRequestRejectsNonImageFallbackMIME(t *testing.T) {
	nonImageUpload := OpenAIImagesUpload{
		FileName:    "not-an-image.bin",
		ContentType: "application/octet-stream",
		Data:        []byte("plain text"),
	}

	tests := []struct {
		name   string
		parsed *OpenAIImagesRequest
	}{
		{
			name: "input upload",
			parsed: &OpenAIImagesRequest{
				Endpoint: openAIImagesEditsEndpoint,
				Prompt:   "edit the image",
				Uploads:  []OpenAIImagesUpload{nonImageUpload},
			},
		},
		{
			name: "mask upload",
			parsed: &OpenAIImagesRequest{
				Endpoint:       openAIImagesEditsEndpoint,
				Prompt:         "edit the image",
				InputImageURLs: []string{"https://example.com/input.png"},
				MaskUpload:     &nonImageUpload,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, err := buildOpenAIImagesResponsesRequest(tt.parsed, "gpt-image-2")
			require.EqualError(t, err, `upload "not-an-image.bin" is not an image`)
			require.Nil(t, body)
		})
	}
}
