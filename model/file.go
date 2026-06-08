package model

import (
	"github.com/jinzhu/gorm"
	_ "github.com/jinzhu/gorm/dialects/sqlite"
	"go-file/common"
	"os"
	"path"
	"strings"
)

type File struct {
	Id              int    `json:"id"`
	Filename        string `json:"filename"`
	Description     string `json:"description"`
	Uploader        string `json:"uploader"`
	Link            string `json:"link" gorm:"unique"`
	Time            string `json:"time"`
	DownloadCounter int    `json:"download_counter"`
	// Size is the file size in bytes, recorded at upload time. Used to verify
	// integrity when archiving to / restoring from cold storage.
	Size int64 `json:"size"`
	// StorageState is "" / "local" when the bytes live on local disk, or
	// "archived" when they have been moved to OSS. An empty value is treated as
	// local for backward compatibility with rows created before this feature.
	StorageState string `json:"storage_state"`
	// OSSKey is the object key in the bucket once archived.
	OSSKey string `json:"oss_key"`
	// LastAccess records the last download/preview time of an archived-then-
	// restored file, used to decide when to re-archive it.
	LastAccess string `json:"last_access"`
}

// IsArchived reports whether the file's bytes currently live in cold storage.
func (file *File) IsArchived() bool {
	return file.StorageState == common.StorageArchived
}

type LocalFile struct {
	Name         string
	Link         string
	Size         string
	IsFolder     bool
	ModifiedTime string
}

func AllFiles() ([]*File, error) {
	var files []*File
	var err error
	err = DB.Find(&files).Error
	return files, err
}

func QueryFiles(query string, startIdx int) ([]*File, error) {
	var files []*File
	var err error
	query = strings.ToLower(query)
	err = DB.Limit(common.ItemsPerPage).Offset(startIdx).Where("filename LIKE ? or description LIKE ? or uploader LIKE ? or time LIKE ?", "%"+query+"%", "%"+query+"%", "%"+query+"%", "%"+query+"%").Order("id desc").Find(&files).Error
	return files, err
}

func (file *File) Insert() error {
	var err error
	err = DB.Create(file).Error
	return err
}

// Delete Make sure link is valid! Because we will use os.Remove to delete it!
func (file *File) Delete() error {
	if err := DB.Delete(file).Error; err != nil {
		return err
	}
	// Best-effort: remove the cold copy too so archived files do not leak storage
	// in OSS after the metadata row is gone.
	if file.IsArchived() && file.OSSKey != "" {
		if oss, err := common.NewOSSClient(); err != nil {
			common.SysError("skip OSS delete, client unavailable: " + err.Error())
		} else if err := oss.DeleteObject(file.OSSKey); err != nil {
			common.SysError("failed to delete archived object " + file.OSSKey + ": " + err.Error())
		}
	}
	// The local copy may legitimately be absent for an archived file.
	if err := os.Remove(path.Join(common.UploadPath, file.Link)); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func UpdateDownloadCounter(link string) {
	DB.Model(&File{}).Where("link = ?", link).UpdateColumn("download_counter", gorm.Expr("download_counter + 1"))
}

// FileByLink loads a single file row by its unique link, or nil when not found.
func FileByLink(link string) (*File, error) {
	var file File
	err := DB.Where("link = ?", link).First(&file).Error
	if gorm.IsRecordNotFoundError(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &file, nil
}

// LocalFilesForArchive returns files whose bytes still live on local disk
// (StorageState empty or "local"), oldest first, so the worker can decide which
// have aged past the archive threshold.
func LocalFilesForArchive() ([]*File, error) {
	var files []*File
	err := DB.Where("storage_state = ? OR storage_state = ? OR storage_state IS NULL",
		"", common.StorageLocal).Order("id asc").Find(&files).Error
	return files, err
}

// MarkArchived flips a file to the archived state after its bytes are in OSS.
func (file *File) MarkArchived(ossKey string, size int64) error {
	return DB.Model(&File{}).Where("id = ?", file.Id).Updates(map[string]interface{}{
		"storage_state": common.StorageArchived,
		"oss_key":       ossKey,
		"size":          size,
	}).Error
}

// MarkRestored flips a file back to local and stamps the access time, which the
// re-archive clock counts from.
func (file *File) MarkRestored(accessTime string) error {
	return DB.Model(&File{}).Where("id = ?", file.Id).Updates(map[string]interface{}{
		"storage_state": common.StorageLocal,
		"last_access":   accessTime,
	}).Error
}
