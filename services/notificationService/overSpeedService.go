package notificationServices

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	lineService "ezview.com/engine/notifications/services/lineService"
	logService "ezview.com/engine/notifications/services/logService"
	telegramService "ezview.com/engine/notifications/services/telegramService"
	userService "ezview.com/engine/notifications/services/userService"
	"ezview.com/engine/notifications/services/utilService"
	"github.com/joho/godotenv"
)

type APIResponse struct {
	Success   bool        `json:"success"`
	Status    int         `json:"status"`
	Timestamp string      `json:"timestamp"`
	Message   string      `json:"message"`
	Data      []OverSpeed `json:"data"`
}

// โครงสร้างข้อมูลการแจ้งเตือน
type OverSpeed struct {
	VehicleID              int    `json:"vehicleId"`
	LicenseNo              string `json:"licenseNo"`
	GPSDateTime            string `json:"gpsDateTime"`
	Latitude               string `json:"latitude"`
	Longitude              string `json:"longitude"`
	Speed                  int    `json:"speed"`
	LocationTH             string `json:"locationTH"`
	LocationEN             string `json:"locationEN"`
	CustomerName           string `json:"customerName"`
	SmartcardDCode         string `json:"smartcardDCode"`
	DriverName             string `json:"driverName"`
	MainGroupName          string `json:"mainGroupName"`
	SpeedLimit             int    `json:"speedLimit"`
	InOutWaypointStatus    int    `json:"inOutWaypointStatus"`
	EngineOnDateTime       string `json:"engineOnDateTime"`
	EngineOffDateTime      string `json:"engineOffDateTime"`
	WaypointLocationNameTH string `json:"waypointLocationNameTH"`
	WaypointLocationNameEN string `json:"waypointLocationNameEN"`
	IoStatus               int    `json:"ioStatus"`
	IoNumber               int    `json:"ioNumber"`
	IoName                 string `json:"ioName"`
}

func OverSpeedNotification(startTime time.Time, settings map[string]interface{}, userNotifySetting *[]userService.UserNotifySetting) {
	// กำหนดค่า default ให้กับ lastDateTime หากไม่มี LastMessageGPSDateTime
	var totalNotificationsSent int
	var totalErrors int

	for i, user := range *userNotifySetting {
		var lastDateTime time.Time

		// ตรวจสอบว่า user มีค่า LastMessageGPSDateTime หรือไม่
		if user.LastMessageGPSDateTime.IsZero() {
			// หากไม่มีค่า ใช้ startTime ย้อนหลัง 5 นาที
			lastDateTime = startTime.Add(-5 * time.Minute)
		} else {
			// หากมีค่า ใช้ LastMessageGPSDateTime จาก UserNotifySetting
			lastDateTime = user.LastMessageGPSDateTime
		}

		// Fetch over-speed notifications for the user
		overSpeedNotifications, err := getOverSpeedNotificationsByUserId(user.UserID, lastDateTime.Format(time.RFC3339))
		if err != nil {
			logService.WriteLog("over_speed_error", fmt.Sprintf("Failed to fetch notifications for UserID %d: %v", user.UserID, err))
			continue
		}

		// ตรวจสอบว่ามีการแจ้งเตือนใน overSpeedNotifications หรือไม่
		if len(overSpeedNotifications) > 0 {
			// อัปเดต `LastMessageGPSDateTime` ด้วยเวลาของการแจ้งเตือนล่าสุด
			latestNotification := overSpeedNotifications[len(overSpeedNotifications)-1]
			// Correct time format for parsing the input
			layout := "02/01/2006 15:04:05" // This matches "16/01/2025 14:43:11"

			// Try parsing the time with the correct layout
			parsedTime, err := time.Parse(layout, latestNotification.GPSDateTime)
			if err != nil {
				logService.WriteLog("over_speed_error", fmt.Sprintf("Failed to parse GPSDateTime for UserID %d: %v", user.UserID, err))
				continue
			}
			(*userNotifySetting)[i].LastMessageGPSDateTime = parsedTime
		}

		// จัดรูปแบบข้อความการแจ้งเตือน
		messages := formatNotifications(overSpeedNotifications)

		// ส่งข้อความผ่าน Telegram
		if user.IsSendTelegram {
			telegramSettings := make([]telegramService.TelegramSetting, len(user.TelegramSetting))
			for i, setting := range user.TelegramSetting {
				telegramSettings[i] = telegramService.TelegramSetting{
					BotID:   setting.BotID,
					ChatIDs: setting.ChatIDs,
				}
			}
			telegramSent, telegramErrors := telegramService.SendToTelegram(telegramSettings, messages)
			totalNotificationsSent += telegramSent
			totalErrors += telegramErrors
		}

		// ส่งข้อความผ่าน Line
		if user.IsSendLine {
			lineSettings := make([]lineService.LineSetting, len(user.LineSetting))
			for i, setting := range user.LineSetting {
				lineSettings[i] = setting.(lineService.LineSetting)
			}
			lineSent, lineErrors := lineService.SendToLine(lineSettings, messages)
			totalNotificationsSent += lineSent
			totalErrors += lineErrors
		}
	}

	// บันทึกจำนวนข้อความที่ส่งและข้อผิดพลาดใน log
	finalLogMessage := fmt.Sprintf("Total notifications sent: %d, Total errors: %d", totalNotificationsSent, totalErrors)
	logService.WriteLog("over_speed", finalLogMessage)
}

// ฟังก์ชันจัดรูปแบบข้อความแจ้งเตือน
func formatNotifications(notifications []OverSpeed) []string {
	var messages []string
	for _, notification := range notifications {
		message := fmt.Sprintf(
			"แจ้งเตือนความเร็วเกินกำหนด %d กม./ชม.\n"+
				"ชื่อ: %s\n"+
				"เลขทะเบียน: %s\n"+
				"คนขับ: %s\n"+
				"วันเวลา: %s\n"+
				"ความเร็ว: %d กม./ชม.\n"+
				"สถานที่: %s\n"+
				"https://www.google.com/maps?q=%s,%s",
			notification.SpeedLimit,
			notification.CustomerName,
			notification.LicenseNo,
			utilService.NonEmptyString(notification.DriverName, "-"),
			notification.GPSDateTime,
			notification.Speed,
			notification.LocationTH,
			notification.Latitude,
			notification.Longitude,
		)
		messages = append(messages, message)
	}
	return messages
}

func getOverSpeedNotificationsByUserId(userId int, lastDateTime string) ([]OverSpeed, error) {
	// Load .env file
	err := godotenv.Load()
	if err != nil {
		return nil, fmt.Errorf("failed to load .env file: %v", err)
	}

	// Get API URL from .env
	apiURL := os.Getenv("API_URL")
	if apiURL == "" {
		return nil, fmt.Errorf("API_URL is not set in .env")
	}

	// Get Authorization token from .env
	authToken := os.Getenv("JWT_TOKEN")
	if authToken == "" {
		return nil, fmt.Errorf("JWT_TOKEN is not set in .env")
	}

	// Define the request body
	requestBody := map[string]interface{}{
		"LastdateTime":         lastDateTime, // "2025-01-15T10:00:56Z"
		"userId":               userId,
		"messageType":          []string{"V", "19", "123"},
		"eventId":              6010,
		"deleteLogMessageType": []string{"V", "19", "123"},
	}
	bodyBytes, err := json.Marshal(requestBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request body: %v", err)
	}

	// Create the HTTP request
	req, err := http.NewRequest("POST", fmt.Sprintf("%s/notifications/vehicle-notification", apiURL), bytes.NewBuffer(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request: %v", err)
	}

	// Set headers
	req.Header.Set("accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", authToken))

	// Retry logic
	var resp *http.Response

	maxRetries := 5

	for i := 0; i < maxRetries; i++ {
		client := &http.Client{}
		resp, err = client.Do(req)

		if err != nil {
			if i == maxRetries-1 {
				return nil, fmt.Errorf("error sending to Over Speed Notifications after %d retries: %w", maxRetries, err)	
			}
			time.Sleep(time.Second * time.Duration(i+1)) // หน่วงเวลาเพิ่มขึ้นตามรอบที่พยายาม
			continue
		}
		defer resp.Body.Close()
	
		// Check HTTP status
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("failed to get data from API, status code: %d", resp.StatusCode)
		}
	
		// Decode JSON response into APIResponse
		var apiResponse APIResponse
		if err := json.NewDecoder(resp.Body).Decode(&apiResponse); err != nil {
			return nil, fmt.Errorf("failed to decode JSON response: %v", err)
		}
	
		// Check if the API response indicates success
		if !apiResponse.Success {
			return nil, fmt.Errorf("API response error: %s", apiResponse.Message)
		}
	
		return apiResponse.Data, nil
	}

	return nil, fmt.Errorf("failed to get data from API after %d retries", maxRetries)
}
