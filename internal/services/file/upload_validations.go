package file

/* Validation helpers split from upload_service.go (no behavior change). */

import (
	"bytes"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"path/filepath"
	"pixelpunk/internal/models"
	"pixelpunk/internal/services/setting"
	"pixelpunk/pkg/common"
	"pixelpunk/pkg/database"
	"pixelpunk/pkg/errors"
	"pixelpunk/pkg/logger"
	"strings"
	"time"

	"gorm.io/gorm"
)

func validateUploadInput(ctx *UploadContext) error {
	resolvedDuration, err := ResolveStorageDurationForUser(ctx.UserID, ctx.StorageDuration)
	if err != nil {
		return err
	}
	ctx.StorageDuration = resolvedDuration

	if err := EnforceUploadPoliciesForRequest(ctx.UserID, ctx.StorageDuration); err != nil {
		return err
	}

	settingsMap, err := setting.GetSettingsByGroupAsMap("upload")
	maxFileSize := int64(20 * 1024 * 1024) // 默认20MB

	if err == nil {
		if maxSizeVal, ok := settingsMap.Settings["max_file_size"]; ok {
			if maxSizeMB, ok := maxSizeVal.(float64); ok {
				maxFileSize = int64(maxSizeMB * 1024 * 1024) // 转换MB为字节
			}
		}
	}

	if maxFileSize > 0 && ctx.File.Size > maxFileSize {
		maxSizeMB := maxFileSize / (1024 * 1024)
		return errors.New(errors.CodeFileTooLarge, fmt.Sprintf("文件大小不能超过%dMB", maxSizeMB))
	}

	fileExt := strings.ToLower(filepath.Ext(ctx.File.Filename))
	ctx.FileExt = fileExt

	if !isValidFileType(fileExt) {
		return errors.New(errors.CodeFileTypeNotSupported, "当前格式不被支持、请联系管理员解除限制！")
	}

	if err := validateFileContentType(ctx.File, fileExt); err != nil {
		return err
	}

	if ctx.FolderID == "null" {
		ctx.FolderID = ""
	}
	if ctx.FolderID != "" {
		return validateFolder(ctx)
	}

	if ctx.StorageDuration != "" {
		storageConfig, err := setting.CreateStorageConfig()
		if err != nil {
			logger.Warn("获取存储配置失败，使用默认配置: %v", err)
			storageConfig = common.CreateDefaultStorageConfig()
		}
		if err := storageConfig.ValidateStorageDuration(ctx.StorageDuration, ctx.IsGuestUpload); err != nil {
			return errors.Wrap(err, errors.CodeInvalidParameter, err.Error())
		}
	}
	return nil
}

func isValidFileType(ext string) bool {
	settingsMap, err := setting.GetSettingsByGroupAsMap("upload")
	if err != nil {
		logger.Warn("获取文件格式设置失败，使用默认配置: %v", err)
		validTypes := map[string]bool{
			".jpg": true, ".jpeg": true, ".png": true, ".gif": true, ".webp": true,
			".bmp": true, ".apng": true, ".svg": true, ".ico": true, ".jp2": true,
			".tiff": true, ".tif": true, ".tga": true, ".heic": true, ".heif": true,
		}
		return validTypes[ext]
	}
	if formatsInterface, ok := settingsMap.Settings["allowed_file_formats"]; ok {
		if formats, ok := formatsInterface.([]any); ok {
			extWithoutDot := strings.TrimPrefix(ext, ".")
			for _, format := range formats {
				if formatStr, ok := format.(string); ok && formatStr == extWithoutDot {
					return true
				}
			}
			return false
		}
	}
	validTypes := map[string]bool{
		".jpg": true, ".jpeg": true, ".png": true, ".gif": true, ".webp": true,
		".bmp": true, ".apng": true, ".svg": true, ".ico": true, ".jp2": true,
		".tiff": true, ".tif": true, ".tga": true, ".heic": true, ".heif": true,
	}
	return validTypes[ext]
}

func validateFileContentType(file *multipart.FileHeader, ext string) error {
	f, err := file.Open()
	if err != nil {
		return errors.Wrap(err, errors.CodeFileUploadFailed, "读取上传文件失败")
	}
	defer f.Close()

	header := make([]byte, 512)
	n, err := io.ReadFull(f, header)
	if err != nil && err != io.ErrUnexpectedEOF && err != io.EOF {
		return errors.Wrap(err, errors.CodeFileUploadFailed, "读取文件头失败")
	}
	header = header[:n]

	detected := strings.ToLower(http.DetectContentType(header))
	if isMimeAllowed(ext, detected, header) {
		return nil
	}

	return errors.New(errors.CodeFileTypeNotSupported, fmt.Sprintf("文件内容与扩展名不匹配: ext=%s mime=%s", ext, detected))
}

func isMimeAllowed(ext, mime string, header []byte) bool {
	// 部分格式（如 svg/heic/ico/tga）无法稳定通过 DetectContentType 识别，补充魔数和内容特征判断
	switch ext {
	case ".jpg", ".jpeg":
		return strings.HasPrefix(mime, "image/jpeg") || hasJPEGMagic(header)
	case ".png", ".apng":
		return strings.HasPrefix(mime, "image/png") || hasPNGMagic(header)
	case ".gif":
		return strings.HasPrefix(mime, "image/gif") || hasGIFMagic(header)
	case ".webp":
		return strings.HasPrefix(mime, "image/webp") || hasWEBPMagic(header)
	case ".bmp":
		return strings.HasPrefix(mime, "image/bmp") || hasBMPMagic(header)
	case ".svg":
		if strings.HasPrefix(mime, "image/svg+xml") || strings.HasPrefix(mime, "text/xml") || strings.HasPrefix(mime, "application/xml") {
			return bytes.Contains(bytes.ToLower(header), []byte("<svg"))
		}
		return bytes.Contains(bytes.ToLower(header), []byte("<svg"))
	case ".ico":
		return strings.HasPrefix(mime, "image/x-icon") || strings.HasPrefix(mime, "image/vnd.microsoft.icon") || hasICOMagic(header)
	case ".jp2":
		return strings.Contains(mime, "jp2") || hasJP2Magic(header)
	case ".tif", ".tiff":
		return strings.HasPrefix(mime, "image/tiff") || hasTIFFMagic(header)
	case ".heic", ".heif":
		return strings.HasPrefix(mime, "image/heic") || strings.HasPrefix(mime, "image/heif") || hasHEIFMagic(header)
	case ".tga":
		// TGA 常被识别为 octet-stream，允许基础头部特征
		return strings.HasPrefix(mime, "application/octet-stream") || looksLikeTGA(header)
	default:
		return false
	}
}

func hasJPEGMagic(b []byte) bool {
	return len(b) >= 3 && b[0] == 0xFF && b[1] == 0xD8 && b[2] == 0xFF
}

func hasPNGMagic(b []byte) bool {
	return len(b) >= 8 && bytes.Equal(b[:8], []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A})
}

func hasGIFMagic(b []byte) bool {
	return len(b) >= 6 && (bytes.Equal(b[:6], []byte("GIF87a")) || bytes.Equal(b[:6], []byte("GIF89a")))
}

func hasWEBPMagic(b []byte) bool {
	return len(b) >= 12 && bytes.Equal(b[:4], []byte("RIFF")) && bytes.Equal(b[8:12], []byte("WEBP"))
}

func hasBMPMagic(b []byte) bool {
	return len(b) >= 2 && b[0] == 'B' && b[1] == 'M'
}

func hasICOMagic(b []byte) bool {
	return len(b) >= 4 && b[0] == 0x00 && b[1] == 0x00 && b[2] == 0x01 && b[3] == 0x00
}

func hasJP2Magic(b []byte) bool {
	return len(b) >= 12 && bytes.Equal(b[4:8], []byte("jP  "))
}

func hasTIFFMagic(b []byte) bool {
	return len(b) >= 4 && ((b[0] == 'I' && b[1] == 'I' && b[2] == 0x2A && b[3] == 0x00) || (b[0] == 'M' && b[1] == 'M' && b[2] == 0x00 && b[3] == 0x2A))
}

func hasHEIFMagic(b []byte) bool {
	if len(b) < 12 {
		return false
	}
	if !bytes.Equal(b[4:8], []byte("ftyp")) {
		return false
	}
	brand := string(b[8:12])
	return brand == "heic" || brand == "heix" || brand == "hevc" || brand == "hevx" || brand == "mif1" || brand == "msf1"
}

func looksLikeTGA(b []byte) bool {
	// TGA 头至少18字节，末尾 image type 常见值: 2(真彩), 3(灰度), 10(RLE真彩), 11(RLE灰度)
	if len(b) < 18 {
		return false
	}
	imageType := b[2]
	return imageType == 2 || imageType == 3 || imageType == 10 || imageType == 11
}

func validateFolder(ctx *UploadContext) error {
	if ctx.FolderID == "" {
		return nil
	}
	var folder models.Folder
	if err := database.DB.Where("id = ? AND user_id = ?", ctx.FolderID, ctx.UserID).First(&folder).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return errors.New(errors.CodeFolderNotFound, "文件夹不存在")
		}
		return errors.Wrap(err, errors.CodeDBQueryFailed, "查询文件夹失败")
	}
	return nil
}

func validateBatchUploadFiles(files []*multipart.FileHeader) error {
	settingsMap, err := setting.GetSettingsByGroupAsMap("upload")
	maxFileSize := int64(20 * 1024 * 1024)   // 默认20MB单文件限制
	maxBatchSize := int64(100 * 1024 * 1024) // 默认100MB批量限制
	if err == nil {
		if maxSizeVal, ok := settingsMap.Settings["max_file_size"]; ok {
			if maxSizeMB, ok := maxSizeVal.(float64); ok {
				maxFileSize = int64(maxSizeMB * 1024 * 1024)
			}
		}
		if maxBatchSizeVal, ok := settingsMap.Settings["max_batch_size"]; ok {
			if maxBatchSizeMB, ok := maxBatchSizeVal.(float64); ok {
				maxBatchSize = int64(maxBatchSizeMB * 1024 * 1024)
			}
		}
	}
	var totalSize int64
	for _, file := range files {
		if maxFileSize > 0 && file.Size > maxFileSize {
			maxSizeMB := maxFileSize / (1024 * 1024)
			return errors.New(errors.CodeFileTooLarge, fmt.Sprintf("文件%s大小超过单文件限制%dMB", file.Filename, maxSizeMB))
		}
		totalSize += file.Size
	}
	if maxBatchSize > 0 && totalSize > maxBatchSize {
		maxSizeMB := maxBatchSize / (1024 * 1024)
		return errors.New(errors.CodeFileTooLarge, fmt.Sprintf("批量上传总大小不能超过%dMB", maxSizeMB))
	}
	return nil
}

func checkDailyUploadLimit(userID uint, uploadCount int) (bool, error) {
	settingsMap, err := setting.GetSettingsByGroupAsMap("upload")
	if err != nil {
		return false, err
	}
	var dailyLimit int = 50 // 默认值
	if limitVal, ok := settingsMap.Settings["daily_upload_limit"]; ok {
		if limit, ok := limitVal.(float64); ok {
			dailyLimit = int(limit)
		}
	}
	if dailyLimit == -1 {
		return false, nil
	}
	db := database.DB
	var todayCount int64
	startOfDay := time.Now().Truncate(24 * time.Hour)
	endOfDay := startOfDay.Add(24 * time.Hour).Add(-time.Second)
	err = db.Model(&models.File{}).Where("user_id = ? AND created_at BETWEEN ? AND ?", userID, startOfDay, endOfDay).Count(&todayCount).Error
	if err != nil {
		return false, err
	}
	return int(todayCount)+uploadCount > dailyLimit, nil
}
