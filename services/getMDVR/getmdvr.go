package getmdvr

import (
	"bytes"
	"crypto/cipher"
	"crypto/des"
	"crypto/md5"
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

)

// / ECBEncrypter ใช้สำหรับการเข้ารหัสแบบ ECB (Electronic Codebook)
type ECBEncrypter struct {
	b cipher.Block
}

// NewECBEncrypter สร้างตัวใหม่สำหรับการเข้ารหัสแบบ ECB
func NewECBEncrypter(b cipher.Block) *ECBEncrypter {
	return &ECBEncrypter{b: b}
}

// CryptBlocks สำหรับการเข้ารหัสข้อมูลในแบบ ECB
func (x *ECBEncrypter) CryptBlocks(dst, src []byte) {
	if len(src)%x.b.BlockSize() != 0 {
		panic("ECBEncrypter: input not full blocks")
	}
	for i := 0; i < len(src); i += x.b.BlockSize() {
		x.b.Encrypt(dst[i:i+x.b.BlockSize()], src[i:i+x.b.BlockSize()])
	}
}

// GetAIMDVRLink สร้างลิงค์สำหรับการแจ้งเตือน MDVR
func GetAIMDVRLink(CscUserId, VehicleId, DateTime, ViolationId, NotifyType string, reserve string) (string, error) {
	
	const layout = "02/01/2006 15:04:05"
	dt, err := time.Parse(layout, DateTime)
	if err != nil {
		return "", fmt.Errorf("invalid date format: %v", err)
	}

	// เข้ารหัสข้อมูลด้วย MD5
	uid := EncryptMD5(CscUserId)
	vid := EncryptMD5(VehicleId)
	dtStr := EncryptMD5(dt.Format("20060102150405"))
	typeID := EncryptMD5(ViolationId)
	topic := EncryptMD5(NotifyType)

	// ตรวจสอบค่าผลลัพธ์ที่ได้จากการเข้ารหัส
	if uid == "" || vid == "" || dtStr == "" || typeID == "" || topic == "" {
		return "", fmt.Errorf("one or more encrypted values are empty")
	}

	// ดึงค่า URL จาก environment variable
	HowenVideoURL := os.Getenv("HOWEN_VIDEO_URL")
	if HowenVideoURL == "" {
		return "", fmt.Errorf("HOWEN_VIDEO_URL is not set in .env")
	}

	baseURL := HowenVideoURL
	if reserve == "MT_AI" {
		baseURL += "FatigueVideo?"
	} else {
		baseURL += "NotifyMDVR?"
	}

	// สร้าง URL สำหรับการแจ้งเตือน MDVR
	// baseURL := HowenVideoURL + "NotifyMDVR?"
	variable := fmt.Sprintf("uid=%s&vid=%s&as=%s&dt=%s&at=%s&lng=th", uid, vid, topic, dtStr, typeID)

	fullURL := baseURL + variable

	return fullURL, nil
}

// EncryptMD5 ทำการเข้ารหัสข้อความด้วย Triple DES และ MD5
func EncryptMD5(plaintext string) string {
	hash := "@MappointAsia"

	// สร้าง MD5 hash
	md5Hash := md5.Sum([]byte(hash))
	key := md5Hash[:]

	// Triple DES ต้องการคีย์ขนาด 24 ไบต์
	tripleKey := append(key, key[:8]...)

	// สร้าง Triple DES cipher block
	block, err := des.NewTripleDESCipher(tripleKey)
	if err != nil {
		return ""
	}

	// ทำการเติมข้อมูล (Padding) ให้เต็มตามขนาดบล็อก
	plainBytes := pad([]byte(plaintext), block.BlockSize())

	// เข้ารหัสข้อมูลด้วย ECB mode
	encrypted := make([]byte, len(plainBytes))
	mode := NewECBEncrypter(block)
	mode.CryptBlocks(encrypted, plainBytes)

	// แปลงข้อมูลที่เข้ารหัสเป็น Base64
	encoded := base64.StdEncoding.EncodeToString(encrypted)

	// ทำให้ Base64 สามารถใช้ใน URL ได้
	encoded = strings.ReplaceAll(encoded, " ", "+")
	encoded = strings.ReplaceAll(encoded, "-", "+")
	encoded = strings.ReplaceAll(encoded, "_", "/")
	encoded = strings.ReplaceAll(encoded, "%2B", "+")

	return encoded
}

// pad เติมข้อมูลให้เต็มตามขนาดของบล็อกด้วย PKCS5/PKCS7 padding
func pad(data []byte, blockSize int) []byte {
	padding := blockSize - (len(data) % blockSize)
	padText := bytes.Repeat([]byte{byte(padding)}, padding)
	return append(data, padText...)
}

// Function DecryptMD5
// -------------------

// ECBDecrypter implements Electronic Codebook (ECB) mode decryption
type ECBDecrypter struct {
	b cipher.Block
}

func NewECBDecrypter(b cipher.Block) *ECBDecrypter {
	return &ECBDecrypter{b: b}
}

func (x *ECBDecrypter) CryptBlocks(dst, src []byte) {
	if len(src)%x.b.BlockSize() != 0 {
		panic("ECBDecrypter: input not full blocks")
	}
	for i := 0; i < len(src); i += x.b.BlockSize() {
		x.b.Decrypt(dst[i:i+x.b.BlockSize()], src[i:i+x.b.BlockSize()])
	}
}

// DecryptMD5 decrypts the given encrypted string using MD5 and Triple DES
func DecryptMD5(encrypted string) (string, error) {
	hash := "@MappointAsia"

	// URL Decode in case it's URL-encoded
	decodedURL, err := url.QueryUnescape(encrypted)
	if err != nil {
		return "", err
	}

	// Replace URL-safe Base64 encoding
	decodedURL = strings.ReplaceAll(decodedURL, " ", "+")
	decodedURL = strings.ReplaceAll(decodedURL, "-", "+")
	decodedURL = strings.ReplaceAll(decodedURL, "_", "/")

	// Decode Base64 string
	data, err := base64.StdEncoding.DecodeString(decodedURL)
	if err != nil {
		return "", err
	}

	// Create MD5 hash
	md5Hash := md5.Sum([]byte(hash))
	key := md5Hash[:]

	// Triple DES requires a 24-byte key, so extend it
	tripleKey := append(key, key[:8]...)

	// Create Triple DES cipher block
	block, err := des.NewTripleDESCipher(tripleKey)
	if err != nil {
		return "", err
	}

	// Decrypt using ECB mode
	if len(data)%block.BlockSize() != 0 {
		return "", errors.New("invalid encrypted data length")
	}

	decrypted := make([]byte, len(data))
	mode := NewECBDecrypter(block)
	mode.CryptBlocks(decrypted, data)

	// Remove padding (PKCS5/PKCS7)
	decrypted = unpad(decrypted)

	return string(decrypted), nil
}

// Unpad removes PKCS5/PKCS7 padding
func unpad(data []byte) []byte {
	padding := int(data[len(data)-1])
	if padding > len(data) {
		return data
	}
	return data[:len(data)-padding]
}
