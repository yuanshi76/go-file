package controller

import (
	"fmt"
	"go-file/common"
	"go-file/model"
	"os"
	"path/filepath"
	"time"
)

const (
	// archiveScanInterval is how often the worker sweeps for files to archive.
	archiveScanInterval = time.Hour
	// archiveStartupDelay lets the server finish booting before the first sweep.
	archiveStartupDelay = time.Minute
	// fileTimeLayout matches the timestamp format stored on File.Time.
	fileTimeLayout = "2006-01-02 15:04:05"
)

// StartArchiveWorker launches the periodic cold-storage archival loop in a
// background goroutine. Each tick is a no-op while archiving is disabled or
// misconfigured, so the admin can toggle it at runtime without a restart.
func StartArchiveWorker() {
	go func() {
		time.Sleep(archiveStartupDelay)
		for {
			runArchiveSweep()
			time.Sleep(archiveScanInterval)
		}
	}()
}

// runArchiveSweep moves every local file that has aged past the threshold to OSS.
func runArchiveSweep() {
	if !common.ArchiveReady() {
		return
	}
	files, err := model.LocalFilesForArchive()
	if err != nil {
		common.SysError("archive sweep: failed to list files: " + err.Error())
		return
	}
	oss, err := common.NewOSSClient()
	if err != nil {
		common.SysError("archive sweep: " + err.Error())
		return
	}
	cutoff := time.Now().Add(-time.Duration(common.ArchiveAfterDaysOrDefault()) * 24 * time.Hour)
	for _, file := range files {
		if archiveReferenceTime(file).After(cutoff) {
			continue
		}
		if err := archiveOne(oss, file); err != nil {
			common.SysError(fmt.Sprintf("archive %s failed: %s", file.Link, err.Error()))
			continue
		}
		common.SysLog("archived to OSS: " + file.Link)
	}
}

// archiveReferenceTime is the moment the re-archive clock counts from: the last
// access time when present, otherwise the upload time. A timestamp that cannot
// be parsed yields "now" so the file is skipped rather than archived blindly.
func archiveReferenceTime(file *model.File) time.Time {
	raw := file.LastAccess
	if raw == "" {
		raw = file.Time
	}
	t, err := time.ParseInLocation(fileTimeLayout, raw, time.Local)
	if err != nil {
		return time.Now()
	}
	return t
}

// archiveOne uploads a single file to OSS, verifies the remote size, then drops
// the local copy. It never deletes local bytes unless the OSS copy is confirmed.
func archiveOne(oss *common.OSSClient, file *model.File) error {
	localPath := filepath.Join(common.UploadPath, file.Link)
	info, err := os.Stat(localPath)
	if err != nil {
		return fmt.Errorf("stat local file: %w", err)
	}
	key := common.JoinKey(common.OSSKeyPrefix, file.Link)
	if err := oss.PutObjectFromFile(key, localPath); err != nil {
		return err
	}
	size, exists, err := oss.HeadObject(key)
	if err != nil {
		return fmt.Errorf("verify upload: %w", err)
	}
	if !exists || size != info.Size() {
		return fmt.Errorf("size mismatch after upload: remote=%d local=%d", size, info.Size())
	}
	if err := file.MarkArchived(key, info.Size()); err != nil {
		return fmt.Errorf("mark archived: %w", err)
	}
	if err := os.Remove(localPath); err != nil && !os.IsNotExist(err) {
		// Non-fatal: the row is already archived; the orphan local file will be
		// overwritten or cleaned up on the next restore.
		common.SysError("archived but failed to remove local copy " + localPath + ": " + err.Error())
	}
	return nil
}

// EnsureLocalCopy guarantees an archived file's bytes are on local disk, pulling
// them back through the (free) WebDAV channel when needed, and refreshes the
// access time so the re-archive clock restarts. It returns the path to serve.
func EnsureLocalCopy(file *model.File) (string, error) {
	localPath := filepath.Join(common.UploadPath, file.Link)
	if !file.IsArchived() {
		return localPath, nil
	}
	if _, err := os.Stat(localPath); err == nil {
		// Bytes are already present locally; just refresh access state.
		if err := file.MarkRestored(time.Now().Format(fileTimeLayout)); err != nil {
			common.SysError("failed to refresh access time for " + file.Link + ": " + err.Error())
		}
		return localPath, nil
	}
	webdav, err := common.NewWebDAVClient()
	if err != nil {
		return "", err
	}
	// Both OSS and WebDAV front the same storage; the relative path under each
	// configured root prefix is the file's link.
	if err := webdav.Download(file.Link, localPath, file.Size); err != nil {
		return "", fmt.Errorf("WebDAV restore failed: %w", err)
	}
	if err := file.MarkRestored(time.Now().Format(fileTimeLayout)); err != nil {
		common.SysError("restored but failed to update state for " + file.Link + ": " + err.Error())
	}
	return localPath, nil
}
