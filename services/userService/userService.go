package userService

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/joho/godotenv"
)

type TelegramSetting struct {
	BotID   string   `json:"botId"`
	ChatIDs []string `json:"chatIds"`
}

type UserNotifySetting struct {
	UserID                 int               `json:"userId"`
	IsSendEmail            bool              `json:"isSendEmail"`
	EmailSetting           []interface{}     `json:"emailSetting"`
	IsSendTelegram         bool              `json:"isSendTelegram"`
	TelegramSetting        []TelegramSetting `json:"telegramSetting"`
	IsSendLine             bool              `json:"isSendLine"`
	LineSetting            []interface{}     `json:"lineSetting"`
	Reserve1               string            `json:"reserve1"`
	Reserve2               string            `json:"reserve2"`
	LastMessageGPSDateTime time.Time         `json:"lastMessageGpsDateTime"`
}

type APIResponse struct {
	Success   bool                `json:"success"`
	Status    int                 `json:"status"`
	Timestamp string              `json:"timestamp"`
	Message   string              `json:"message"`
	Data      []UserNotifySetting `json:"data"`
}

func GetUsersNotifySetting(eventId int) ([]UserNotifySetting, error) {
	// โหลดค่า .env
	err := godotenv.Load()
	if err != nil {
		return nil, fmt.Errorf("failed to load .env file: %v", err)
	}

	// ดึงค่า URL และ JWT จาก .env
	apiURL := os.Getenv("API_URL")
	if apiURL == "" {
		return nil, fmt.Errorf("API_URL is not set in .env")
	}

	jwtToken := os.Getenv("JWT_TOKEN")
	if jwtToken == "" {
		return nil, fmt.Errorf("JWT_TOKEN is not set in .env")
	}

	// สร้าง HTTP Request
	req, err := http.NewRequest("GET", fmt.Sprintf("%s/notifications/user-notification?eventId=%d", apiURL, eventId), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %v", err)
	}

	// เพิ่ม JWT ใน Header
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", jwtToken))

	// ส่ง Request
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %v", err)
	}
	defer resp.Body.Close()

	// ตรวจสอบว่า HTTP status เป็น 200 OK หรือไม่
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get data from API, status code: %d", resp.StatusCode)
	}

	// แปลงข้อมูล JSON จาก API เป็น struct APIResponse
	var apiResponse APIResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResponse); err != nil {
		return nil, fmt.Errorf("failed to decode API response: %v", err)
	}

	// ตรวจสอบว่า success == true
	if !apiResponse.Success {
		return nil, fmt.Errorf("API returned error: %s", apiResponse.Message)
	}

	// คืนค่าเฉพาะฟิลด์ Data
	return apiResponse.Data, nil
}

// updateUsers อัปเดต users โดยไม่ให้มีผู้ใช้ซ้ำ
func UpdateUsers(existingUsers []UserNotifySetting, newUsers []UserNotifySetting) []UserNotifySetting {
	// สร้าง map ของ UserID เพื่อเปรียบเทียบการซ้ำ
	existingUserMap := make(map[int]UserNotifySetting)
	for _, user := range existingUsers {
		existingUserMap[user.UserID] = user
	}

	// เพิ่มผู้ใช้ใหม่ถ้ายังไม่มี
	updatedUsers := existingUsers
	for i, newUser := range newUsers {
		if existingUser, exists := existingUserMap[newUser.UserID]; !exists {
			// หากไม่มีผู้ใช้ใน existingUsers ให้เพิ่มใหม่
			updatedUsers = append(updatedUsers, newUser)
			// พิมพ์ผู้ใช้ที่เพิ่มใหม่
			// fmt.Printf("User added: %d, LastMessageGPSDateTime: %s\n", newUser.UserID, newUser.LastMessageGPSDateTime)
		} else {
			// หากมีผู้ใช้ใน existingUsers ให้เปลี่ยนทุกอย่างยกเว้น LastMessageGPSDateTime
			newUser.LastMessageGPSDateTime = existingUser.LastMessageGPSDateTime
			// อัปเดตข้อมูลผู้ใช้ใน updatedUsers
			updatedUsers[i] = newUser
			// พิมพ์ข้อมูลที่ถูกเปลี่ยนแปลง
			// fmt.Printf("User updated: %d, LastMessageGPSDateTime: %s\n", newUser.UserID, newUser.LastMessageGPSDateTime)
		}
	}

	// ลบผู้ใช้เก่าที่ไม่มีในข้อมูลใหม่
	updatedUserMap := make(map[int]UserNotifySetting)
	for _, newUser := range newUsers {
		updatedUserMap[newUser.UserID] = newUser
	}

	finalUsers := []UserNotifySetting{}
	for _, user := range updatedUsers {
		if _, exists := updatedUserMap[user.UserID]; exists {
			// พิมพ์ผู้ใช้ที่ยังคงอยู่ (ไม่มีการเปลี่ยนแปลง)
			// fmt.Printf("User unchanged: %d, LastMessageGPSDateTime: %s\n", user.UserID, user.LastMessageGPSDateTime)
			finalUsers = append(finalUsers, user)
		}
	}

	return finalUsers
}
