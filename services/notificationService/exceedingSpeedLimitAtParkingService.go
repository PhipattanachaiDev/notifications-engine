package notificationServices

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	"ezview.com/engine/notifications/services/lineService"
	"ezview.com/engine/notifications/services/logService"
	telegramService "ezview.com/engine/notifications/services/telegramService"
	userService "ezview.com/engine/notifications/services/userService"
	"github.com/joho/godotenv"
)

type ExceedSpeedLimitAtParkingAPIResponse struct {
	Success   bool                        `json:"success"`
	Status    int                         `json:"status"`
	Timestamp string                      `json:"timestamp"`
	Message   string                      `json:"message"`
	Data      []ExceedSpeedLimitAtParking `json:"data"`
}

type ExceedSpeedLimitAtParking struct {
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
	NotificationEventId    int    `json:"notificationEventId"`
	EventNameTH            string `json:"eventNameTH"`
	ViolationId            int    `json:"violationId"`
	CscUserId              int    `json:"cscUserId"`
	PoiName                string `json:"poiName"`
}

func ExceedingSpeedLimitAtParking(startTime time.Time, settings map[string]interface{}, userNotifySetting *[]userService.UserNotifySetting) {
	var totalNotificationsSent int
	var totalErrors int

	for i, user := range *userNotifySetting {
		// var lastDateTime time.Time

		// // ตรวจสอบว่า user มีค่า LastMessageGPSDateTime หรือไม่
		// if user.LastMessageGPSDateTime.IsZero() {
		// 	// หากไม่มีค่า ใช้ startTime ย้อนหลัง 5 นาที
		// 	lastDateTime = startTime.Add(-5 * time.Minute)
		// } else {
		// 	// หากมีค่า ใช้ LastMessageGPSDateTime จาก UserNotifySetting
		// 	lastDateTime = user.LastMessageGPSDateTime
		// }

		// Fetch exCeedingSpeedLimit notifications from the user
		exCeedingSpeedLimitNotifications, err := getExceedingSpeedLimitAtParkingNotificationsByUserId(user.UserID)
		if err != nil {
			logService.WriteLog("exceeding_speed_limit_at_parking_error", fmt.Sprintf("failed to get exceeding speed limit at parking notifications for user %d: %v", user.UserID, err))
			continue
		}

		// ตรวจสอบว่ามีการแจ้งเตือนใน exCeedingSpeedLimitNotifications หรือไม่
		if len(exCeedingSpeedLimitNotifications) > 0 {
			// อัพเดท LastMessageGPSDateTime ด้วยเวลาของการแจ้งเตือนล่าสุด
			latestNotification := exCeedingSpeedLimitNotifications[len(exCeedingSpeedLimitNotifications)-1]
			// Correct time format for parsing the input
			layout := "02/01/2006 15:04:05"

			// Try parsing the time with the correct layout
			paredTime, err := time.Parse(layout, latestNotification.GPSDateTime)
			if err != nil {
				logService.WriteLog("exceeding_speed_limit_at_parking_error", fmt.Sprintf("failed to parse time: %v", err))
				continue
			}
			(*userNotifySetting)[i].LastMessageGPSDateTime = paredTime
		}

		// จัดรูปแบบข้อความแจ้งเตือน
		messages := formatExceedingSpeedLimitAtParkingNotifications(exCeedingSpeedLimitNotifications)

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
	logService.WriteLog("exceeding_speed_limit_at_parking", finalLogMessage)
}

func getExceedingSpeedLimitAtParkingNotificationsByUserId(userId int) ([]ExceedSpeedLimitAtParking, error) {
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
		"userId":               userId,
		"eventId":              6019,
	}
	bodyBytes, err := json.Marshal(requestBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request body: %v", err)
	}

	// Create the HTTP request
	req, err := http.NewRequest("POST", fmt.Sprintf("%s/notifications/vehicle-overspeed-inwaypoint-notification", apiURL), bytes.NewBuffer(bodyBytes))
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
				return nil, fmt.Errorf("error sending to Exceeding Speed Limit At Parking Notifications after %d retries: %w", maxRetries, err)	
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
		var apiResponse ExceedSpeedLimitAtParkingAPIResponse
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

func formatExceedingSpeedLimitAtParkingNotifications(notifications []ExceedSpeedLimitAtParking) []string {
	var messages []string
	for _, notification := range notifications {
		message := fmt.Sprintf(
			"แจ้งเตือนรถเข้าจุดจอดที่กำหนดและความเร็วเกิน %d กม./ชม.\n" +
				"เลขทะเบียน: %s\n" +
				"วันเวลา: %s\n" +
				"พนักงานขับรถ: %s\n" +
				"ความเร็ว: %d กม./ชม.\n" +
				"สถานที่: %s\n" +
				"https://www.google.com/maps?q=%s,%s",
		notification.SpeedLimit, notification.LicenseNo, notification.GPSDateTime, notification.DriverName, notification.Speed, notification.LocationTH, notification.Latitude, notification.Longitude)
		messages = append(messages, message)
	}
	return messages
}
