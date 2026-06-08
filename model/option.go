package model

import (
	"errors"
	"go-file/common"
	"strconv"
	"strings"
)

type Option struct {
	Key   string `json:"key" gorm:"primaryKey"`
	Value string `json:"value"`
}

func AllOption() ([]*Option, error) {
	var options []*Option
	var err error
	err = DB.Find(&options).Error
	return options, err
}

func InitOptionMap() {
	common.OptionMap = make(map[string]string)
	common.OptionMap["FileUploadPermission"] = strconv.Itoa(common.FileUploadPermission)
	common.OptionMap["FileDownloadPermission"] = strconv.Itoa(common.FileDownloadPermission)
	common.OptionMap["ImageUploadPermission"] = strconv.Itoa(common.ImageUploadPermission)
	common.OptionMap["ImageDownloadPermission"] = strconv.Itoa(common.ImageDownloadPermission)
	common.OptionMap["VideoDownloadPermission"] = strconv.Itoa(common.VideoDownloadPermission)
	common.OptionMap["MaxUploadSizeMB"] = strconv.Itoa(common.MaxUploadSizeMB)
	// Cold-storage archive settings (non-secret; secrets come from env vars).
	common.OptionMap["ArchiveEnabled"] = strconv.FormatBool(common.ArchiveEnabled)
	common.OptionMap["ArchiveAfterDays"] = strconv.Itoa(common.ArchiveAfterDays)
	common.OptionMap["OSSBucket"] = common.OSSBucket
	common.OptionMap["OSSRegion"] = common.OSSRegion
	common.OptionMap["OSSEndpoint"] = common.OSSEndpoint
	common.OptionMap["OSSAccessKeyID"] = common.OSSAccessKeyID
	common.OptionMap["OSSKeyPrefix"] = common.OSSKeyPrefix
	common.OptionMap["WebDAVBaseURL"] = common.WebDAVBaseURL
	common.OptionMap["WebDAVUsername"] = common.WebDAVUsername
	common.OptionMap["WebDAVRootPrefix"] = common.WebDAVRootPrefix
	common.OptionMap["WebsiteName"] = "Go File"
	common.OptionMap["FooterInfo"] = ""
	common.OptionMap["Version"] = common.Version
	common.OptionMap["Notice"] = ""
	options, _ := AllOption()
	for _, option := range options {
		updateOptionMap(option.Key, option.Value)
	}
}

func UpdateOption(key string, value string) error {
	if key == "MaxUploadSizeMB" {
		n, err := strconv.Atoi(strings.TrimSpace(value))
		if err != nil || n < 0 {
			return errors.New("上传大小限制必须是不小于 0 的整数（0 表示不限制）")
		}
		value = strconv.Itoa(n)
	}
	if key == "ArchiveAfterDays" {
		n, err := strconv.Atoi(strings.TrimSpace(value))
		if err != nil || n <= 0 {
			return errors.New("归档天数必须是大于 0 的整数")
		}
		value = strconv.Itoa(n)
	}
	if key == "ArchiveEnabled" {
		value = strings.TrimSpace(value)
		if value != "true" && value != "false" {
			return errors.New("归档开关只能是 true 或 false")
		}
		if value == "true" && (common.OSSAccessKeySecret() == "" || common.WebDAVPassword() == "") {
			return errors.New("未配置 OSS_ACCESS_KEY_SECRET / WEBDAV_PASSWORD 环境变量，无法启用归档")
		}
	}

	// Save to database first
	option := Option{
		Key:   key,
		Value: value,
	}
	// When updating with struct it will only update non-zero fields by default
	// So we have to use Select here
	if DB.Model(&option).Where("key = ?", key).Update("value", option.Value).RowsAffected == 0 {
		DB.Create(&option)
	}
	// Update OptionMap
	updateOptionMap(key, value)
	return nil
}

func updateOptionMap(key string, value string) {
	common.OptionMap[key] = value
	if strings.HasSuffix(key, "Permission") {
		intValue, _ := strconv.Atoi(value)
		switch key {
		case "FileUploadPermission":
			common.FileUploadPermission = intValue
		case "FileDownloadPermission":
			common.FileDownloadPermission = intValue
		case "ImageUploadPermission":
			common.ImageUploadPermission = intValue
		case "ImageDownloadPermission":
			common.ImageDownloadPermission = intValue
		case "VideoDownloadPermission":
			common.VideoDownloadPermission = intValue
		}
	}
	if key == "MaxUploadSizeMB" {
		if n, err := strconv.Atoi(value); err == nil && n >= 0 {
			common.MaxUploadSizeMB = n
		}
	}
	switch key {
	case "ArchiveEnabled":
		common.ArchiveEnabled = value == "true"
	case "ArchiveAfterDays":
		if n, err := strconv.Atoi(value); err == nil && n > 0 {
			common.ArchiveAfterDays = n
		}
	case "OSSBucket":
		common.OSSBucket = strings.TrimSpace(value)
	case "OSSRegion":
		common.OSSRegion = strings.TrimSpace(value)
	case "OSSEndpoint":
		common.OSSEndpoint = strings.TrimSpace(value)
		// Auto-fill region from the endpoint when the admin left it blank.
		if common.OSSRegion == "" {
			if region := common.InferOSSRegion(common.OSSEndpoint); region != "" {
				common.OSSRegion = region
				common.OptionMap["OSSRegion"] = region
			}
		}
	case "OSSAccessKeyID":
		common.OSSAccessKeyID = strings.TrimSpace(value)
	case "OSSKeyPrefix":
		common.OSSKeyPrefix = strings.Trim(strings.TrimSpace(value), "/")
		common.OptionMap["OSSKeyPrefix"] = common.OSSKeyPrefix
	case "WebDAVBaseURL":
		common.WebDAVBaseURL = strings.TrimSpace(value)
	case "WebDAVUsername":
		common.WebDAVUsername = strings.TrimSpace(value)
	case "WebDAVRootPrefix":
		common.WebDAVRootPrefix = strings.Trim(strings.TrimSpace(value), "/")
		common.OptionMap["WebDAVRootPrefix"] = common.WebDAVRootPrefix
	}
	if key == "StatEnabled" {
		common.StatEnabled = value == "true"
	}
}
