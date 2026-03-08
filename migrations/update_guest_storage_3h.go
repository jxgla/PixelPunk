package migrations

import (
	"fmt"
	"pixelpunk/internal/models"
	"pixelpunk/pkg/logger"

	"gorm.io/gorm"
)

// UpdateGuestStorageTo3Hours 将游客上传时长收敛到 1h/3h，默认 3h。
func UpdateGuestStorageTo3Hours(db *gorm.DB) error {
	logger.Infof("执行迁移: update_guest_storage_to_3h")

	return db.Transaction(func(tx *gorm.DB) error {
		if err := upsertSetting(tx, models.Setting{
			Key:         "guest_allowed_storage_durations",
			Value:       `["1h","3h"]`,
			Type:        "array",
			Group:       "guest",
			Description: "允许的存储时长选项（游客）",
			IsSystem:    true,
		}); err != nil {
			return fmt.Errorf("更新 guest_allowed_storage_durations 失败: %w", err)
		}

		if err := upsertSetting(tx, models.Setting{
			Key:         "guest_default_storage_duration",
			Value:       "3h",
			Type:        "string",
			Group:       "guest",
			Description: "默认存储时长（游客）",
			IsSystem:    true,
		}); err != nil {
			return fmt.Errorf("更新 guest_default_storage_duration 失败: %w", err)
		}

		return nil
	})
}

func upsertSetting(tx *gorm.DB, target models.Setting) error {
	var existing models.Setting
	err := tx.Where("key = ?", target.Key).First(&existing).Error
	if err == nil {
		existing.Value = target.Value
		existing.Type = target.Type
		existing.Group = target.Group
		existing.Description = target.Description
		existing.IsSystem = target.IsSystem
		return tx.Save(&existing).Error
	}

	if err == gorm.ErrRecordNotFound {
		return tx.Create(&target).Error
	}

	return err
}
