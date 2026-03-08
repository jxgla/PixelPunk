package file

import (
	"fmt"
	"os"
	"pixelpunk/internal/models"
	"pixelpunk/pkg/common"
	"pixelpunk/pkg/database"
	"pixelpunk/pkg/errors"
	"strconv"
	"strings"
)

const (
	defaultUploadPauseThresholdGB = 5
)

// EnforceUploadPoliciesForRequest 统一上传策略校验入口。
func EnforceUploadPoliciesForRequest(userID uint, storageDuration string) error {
	return enforceUploadPolicies(userID, storageDuration)
}

// ResolveStorageDurationForUser 根据用户角色解析最终存储时长。
// - 游客：原样返回（由游客规则另行约束）
// - 普通用户：未指定时默认30d
// - 管理员：未指定时保持空（沿用系统默认，可为永久）
func ResolveStorageDurationForUser(userID uint, requested string) (string, error) {
	requested = strings.TrimSpace(requested)
	if userID == 0 {
		return requested, nil
	}

	if requested != "" {
		return requested, nil
	}

	role, err := getUserRole(userID)
	if err != nil {
		return "", err
	}

	if role == common.UserRoleAdmin || role == common.UserRoleSuperAdmin {
		return requested, nil
	}

	return "30d", nil
}

func enforceUploadPolicies(userID uint, storageDuration string) error {
	totalStorage, reachedThreshold, thresholdGB, err := getGlobalStorageStatus()
	if err != nil {
		return err
	}

	if reachedThreshold {
		if !allowAdminOnlyOnThreshold() {
			return errors.New(errors.CodeForbidden, fmt.Sprintf("站点存储已达到 %dGB 上限，上传功能已临时关闭", thresholdGB))
		}

		if err := enforceAdminOnlyWhenThresholdReached(userID); err != nil {
			return err
		}

		_ = totalStorage // 预留给后续日志/监控使用
	}

	if err := enforcePermanentStorageRole(userID, storageDuration); err != nil {
		return err
	}

	return nil
}

func getGlobalStorageStatus() (int64, bool, int, error) {
	thresholdGB := getUploadPauseThresholdGB()
	if thresholdGB <= 0 {
		return 0, false, thresholdGB, nil
	}

	var totalStorage int64
	if err := database.DB.Model(&models.File{}).
		Where("status <> ?", "pending_deletion").
		Select("COALESCE(SUM(size), 0)").
		Scan(&totalStorage).Error; err != nil {
		return 0, false, thresholdGB, errors.Wrap(err, errors.CodeDBQueryFailed, "检查全站存储使用失败")
	}

	thresholdBytes := int64(thresholdGB) * 1024 * 1024 * 1024
	return totalStorage, totalStorage >= thresholdBytes, thresholdGB, nil
}

func enforcePermanentStorageRole(userID uint, storageDuration string) error {
	if strings.TrimSpace(storageDuration) != common.StorageDurationPermanent {
		return nil
	}

	if userID == 0 {
		return errors.New(errors.CodeForbidden, "仅管理员可使用永久存储")
	}

	role, err := getUserRole(userID)
	if err != nil {
		return err
	}

	if role != common.UserRoleSuperAdmin && role != common.UserRoleAdmin {
		return errors.New(errors.CodeForbidden, "仅管理员可使用永久存储，普通用户最多30天")
	}

	return nil
}

func getUserRole(userID uint) (int, error) {
	var user models.User
	if err := database.DB.Select("id", "role").First(&user, userID).Error; err != nil {
		return 0, errors.Wrap(err, errors.CodeDBQueryFailed, "查询用户角色失败")
	}
	return int(user.Role), nil
}

func getUploadPauseThresholdGB() int {
	raw := strings.TrimSpace(os.Getenv("UPLOAD_PAUSE_THRESHOLD_GB"))
	if raw == "" {
		return defaultUploadPauseThresholdGB
	}

	v, err := strconv.Atoi(raw)
	if err != nil || v < 0 {
		return defaultUploadPauseThresholdGB
	}

	return v
}

func allowAdminOnlyOnThreshold() bool {
	raw := strings.TrimSpace(os.Getenv("UPLOAD_ALLOW_ADMIN_ONLY_WHEN_THRESHOLD_REACHED"))
	if raw == "" {
		return true
	}
	v, err := strconv.ParseBool(raw)
	if err != nil {
		return true
	}
	return v
}

func enforceAdminOnlyWhenThresholdReached(userID uint) error {
	if userID == 0 {
		return errors.New(errors.CodeForbidden, "站点存储达到阈值后，仅管理员可上传")
	}

	role, err := getUserRole(userID)
	if err != nil {
		return err
	}

	if role != common.UserRoleAdmin && role != common.UserRoleSuperAdmin {
		return errors.New(errors.CodeForbidden, "站点存储达到阈值后，仅管理员可上传")
	}

	return nil
}
