package model

import (
	"errors"
	_ "github.com/jinzhu/gorm/dialects/sqlite"
	"golang.org/x/crypto/bcrypt"
	"strings"
)

type User struct {
	Id          int    `json:"id"`
	Username    string `json:"username" gorm:"unique;"`
	Password    string `json:"password" gorm:"not null;"`
	DisplayName string `json:"displayName"`
	Role        int    `json:"role" gorm:"type:int;default:1"`   // admin, common
	Status      int    `json:"status" gorm:"type:int;default:1"` // enabled, disabled
	Token       string `json:"token"`
}

// HashPassword returns a bcrypt hash of the given plaintext password.
func HashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

func isBcryptHash(s string) bool {
	return strings.HasPrefix(s, "$2a$") || strings.HasPrefix(s, "$2b$") || strings.HasPrefix(s, "$2y$")
}

func (user *User) Insert() error {
	hashed, err := HashPassword(user.Password)
	if err != nil {
		return err
	}
	user.Password = hashed
	return DB.Create(user).Error
}

func (user *User) Update() error {
	return DB.Model(user).Updates(user).Error
}

func (user *User) Delete() error {
	return DB.Delete(user).Error
}

// IsUsernameTaken reports whether username is already used by another account.
func IsUsernameTaken(username string, excludeId int) bool {
	var count int
	DB.Model(&User{}).Where("username = ? AND id <> ?", username, excludeId).Count(&count)
	return count > 0
}

// UpdateUserFields updates the given columns for the user with the given id.
// A map is used (instead of a struct) so that callers can update an explicit
// set of fields, including clearing them, without GORM's non-zero-field filter.
func UpdateUserFields(id int, fields map[string]interface{}) error {
	if len(fields) == 0 {
		return nil
	}
	return DB.Model(&User{Id: id}).Updates(fields).Error
}

// ValidateAndFill looks up the user by username and verifies the supplied
// plaintext password against the stored bcrypt hash. Legacy plaintext records
// are transparently upgraded to a bcrypt hash on first successful login.
func (user *User) ValidateAndFill() error {
	password := user.Password
	if user.Username == "" || password == "" {
		return errors.New("用户名或密码为空")
	}
	var stored User
	DB.Where("username = ?", user.Username).First(&stored)
	if stored.Id == 0 {
		return errors.New("用户名或密码错误")
	}
	if isBcryptHash(stored.Password) {
		if err := bcrypt.CompareHashAndPassword([]byte(stored.Password), []byte(password)); err != nil {
			return errors.New("用户名或密码错误")
		}
	} else {
		// Legacy plaintext password: compare directly, then upgrade.
		if stored.Password != password {
			return errors.New("用户名或密码错误")
		}
		if hashed, err := HashPassword(password); err == nil {
			DB.Model(&stored).Update("password", hashed)
		}
	}
	*user = stored
	return nil
}

func ValidateUserToken(token string) (user *User) {
	if token == "" {
		return nil
	}
	token = strings.Replace(token, "Bearer ", "", 1)
	user = &User{}
	if DB.Where("token = ?", token).First(user).RowsAffected == 1 {
		return user
	}
	return nil
}
