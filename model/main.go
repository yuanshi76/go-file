package model

import (
	"github.com/google/uuid"
	"github.com/jinzhu/gorm"
	_ "github.com/jinzhu/gorm/dialects/mysql"
	_ "github.com/jinzhu/gorm/dialects/sqlite"
	"go-file/common"
	"os"
)

var DB *gorm.DB

func createAdminAccount() {
	var user User
	DB.Where(User{Role: common.RoleAdminUser}).First(&user)
	if user.Id != 0 {
		// An admin already exists; never reset its credentials.
		return
	}
	password := os.Getenv("ADMIN_PASSWORD")
	generated := false
	if password == "" {
		password = uuid.New().String()[:12]
		generated = true
	}
	hashed, err := HashPassword(password)
	if err != nil {
		common.FatalLog("failed to hash default admin password: " + err.Error())
		return
	}
	admin := User{
		Username:    "admin",
		Password:    hashed,
		Role:        common.RoleAdminUser,
		Status:      common.UserStatusEnabled,
		DisplayName: "Administrator",
	}
	if err := DB.Create(&admin).Error; err != nil {
		common.FatalLog("failed to create default admin account: " + err.Error())
		return
	}
	if generated {
		common.SysLog("================ DEFAULT ADMIN CREATED ================")
		common.SysLog("username: admin")
		common.SysLog("password: " + password)
		common.SysLog("Please log in and change this password immediately.")
		common.SysLog("=======================================================")
	}
}

func CountTable(tableName string) (num int) {
	DB.Table(tableName).Count(&num)
	return
}

func InitDB() (db *gorm.DB, err error) {
	if os.Getenv("SQL_DSN") != "" {
		// Use MySQL
		db, err = gorm.Open("mysql", os.Getenv("SQL_DSN"))
	} else {
		// Use SQLite
		db, err = gorm.Open("sqlite3", common.SQLitePath)
	}
	if err == nil {
		DB = db
		db.AutoMigrate(&File{})
		db.AutoMigrate(&Image{})
		db.AutoMigrate(&User{})
		db.AutoMigrate(&Option{})
		createAdminAccount()
		return DB, err
	} else {
		common.FatalLog("failed to connect to database: " + err.Error())
	}
	return nil, err
}
