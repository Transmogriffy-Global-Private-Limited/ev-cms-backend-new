package customerauth

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/models"
	"gorm.io/gorm"
)

const customerChargerImageDirectory = "uploads"

type CustomerChargerImage struct {
	File        *os.File
	Name        string
	ModifiedAt  time.Time
	ContentType string
}

// OpenCustomerChargerImage authorizes the same published charger boundary as
// the User App detail route before opening only a regular image file from the
// CPO upload directory. The database path is never exposed or used directly.
func (service *Service) OpenCustomerChargerImage(
	ctx context.Context,
	principal Principal,
	publicChargerID string,
) (CustomerChargerImage, error) {
	publicChargerID = strings.ToLower(strings.TrimSpace(publicChargerID))
	if !customerChargerIDPattern.MatchString(publicChargerID) {
		return CustomerChargerImage{}, &APIError{http.StatusBadRequest, "invalid_charger_id", "The charger ID is invalid."}
	}

	var charger models.Charger
	if err := service.database.WithContext(ctx).Preload("Hub").First(
		&charger, "cpo_id = ? AND charger_id = ?", principal.CPOID, publicChargerID,
	).Error; err != nil {
		return CustomerChargerImage{}, customerNetworkNotFound(err, "charger")
	}
	if charger.Hub == nil || charger.Hub.CPOID != principal.CPOID || !charger.Hub.CustomerVisible {
		return CustomerChargerImage{}, customerNetworkNotFound(gorm.ErrRecordNotFound, "charger")
	}

	storedPath := strings.TrimSpace(charger.ChargerImage)
	if storedPath == "" {
		return CustomerChargerImage{}, customerChargerImageNotFound()
	}
	name := filepath.Base(filepath.Clean(storedPath))
	if name == "." || name == string(filepath.Separator) || name == "" {
		return CustomerChargerImage{}, customerChargerImageNotFound()
	}

	file, err := os.Open(filepath.Join(customerChargerImageDirectory, name))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return CustomerChargerImage{}, customerChargerImageNotFound()
		}
		return CustomerChargerImage{}, fmt.Errorf("open customer charger image: %w", err)
	}
	info, err := file.Stat()
	if err != nil {
		file.Close()
		return CustomerChargerImage{}, fmt.Errorf("stat customer charger image: %w", err)
	}
	if !info.Mode().IsRegular() {
		file.Close()
		return CustomerChargerImage{}, customerChargerImageNotFound()
	}

	buffer := make([]byte, 512)
	read, err := file.Read(buffer)
	if err != nil && !errors.Is(err, io.EOF) {
		file.Close()
		return CustomerChargerImage{}, fmt.Errorf("read customer charger image: %w", err)
	}
	contentType := http.DetectContentType(buffer[:read])
	if !allowedCustomerChargerImageType(contentType) {
		file.Close()
		return CustomerChargerImage{}, customerChargerImageNotFound()
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		file.Close()
		return CustomerChargerImage{}, fmt.Errorf("rewind customer charger image: %w", err)
	}

	return CustomerChargerImage{
		File:        file,
		Name:        name,
		ModifiedAt:  info.ModTime(),
		ContentType: contentType,
	}, nil
}

func allowedCustomerChargerImageType(contentType string) bool {
	switch contentType {
	case "image/gif", "image/jpeg", "image/png", "image/webp":
		return true
	default:
		return false
	}
}

func customerChargerImageNotFound() error {
	return &APIError{http.StatusNotFound, "charger_image_not_found", "No image is available for this charger."}
}
