package notificationServices

import (
	"bytes"
	"encoding/json"
	get "ezview.com/engine/notifications/services/getMDVR"
	"ezview.com/engine/notifications/services/lineService"
	"ezview.com/engine/notifications/services/logService"
	telegramService "ezview.com/engine/notifications/services/telegramService"
	userService "ezview.com/engine/notifications/services/userService"
	"fmt"
	"github.com/joho/godotenv"
	"net/http"
	"os"
	"strconv"
	"time"
)

type DrivingAPIResponse struct {
	Success   bool              `json:"success"`
	Status    int               `json:"status"`
	Timestamp string            `json:"timestamp"`
	Message   string            `json:"message"`
	Data      []DrivingBehavior `json:"data"`
}

type DrivingBehavior struct {
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
	ViolationId            int    `json:"notificationEventId"`
	EventNameTH            string `json:"eventNameTH"`
	CscUserId              int    `json:"cscUserId"`
	PoiName                string `json:"poiName"`
}

var lastProcessedTimeDriving = make(map[int]string)

func DrivingBehaviorNotifications(startTime time.Time, settings map[string]interface{}, userNotifySetting *[]userService.UserNotifySetting) {
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

		// Fetch drivingBehavior notifications from the user
		drivingBehaviorNotifications, err := getDrivingBehaviorNotificationsByUserId(user.UserID, lastDateTime.Format(time.RFC3339))
		if err != nil {
			logService.WriteLog("driving_Behavior_error", fmt.Sprintf("failed to get driving behavior notifications for user %d: %v", user.UserID, err))
			continue
		}

		// ตรวจสอบว่ามีการแจ้งเตือนใน drivingBehaviorNotifications หรือไม่
		if len(drivingBehaviorNotifications) > 0 {
			// อัพเดท LastMessageGPSDateTime ด้วยเวลาของการแจ้งเตือนล่าสุด
			latestNotification := drivingBehaviorNotifications[len(drivingBehaviorNotifications)-1]
			// Correct time format for parsing the input
			layout := "02/01/2006 15:04:05"

			// Try parsing the time with the correct layout
			paredTime, err := time.Parse(layout, latestNotification.GPSDateTime)
			if err != nil {
				logService.WriteLog("driving_Behavior_error", fmt.Sprintf("failed to parse time: %v", err))
				continue
			}
			(*userNotifySetting)[i].LastMessageGPSDateTime = paredTime
		}

		for _, driving := range drivingBehaviorNotifications {

			CSCuserIDString := strconv.Itoa(driving.CscUserId)

			vehicleIDString := strconv.Itoa(driving.VehicleID)

			ViolationIDString := strconv.Itoa(driving.ViolationId)

			topic := "DMS"

			// Get MDVR Video URL
			url, err := get.GetAIMDVRLink(CSCuserIDString, vehicleIDString, driving.GPSDateTime, ViolationIDString, topic, user.Reserve1)
			if err != nil {
				logService.WriteLog("driving_Behavior_error", fmt.Sprintf("failed to get MDVR in function driving behavior: %v", err))
				continue
			}

			// จัดรูปแบบข้อความแจ้งเตือน
			messages := formatDrivingBehaviorNotifications([]DrivingBehavior{driving}, url)
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
	}

	// บันทึกจำนวนข้อความที่ส่งและข้อผิดพลาดใน log
	finalLogMessage := fmt.Sprintf("Total notifications sent: %d, Total errors: %d", totalNotificationsSent, totalErrors)
	logService.WriteLog("driving_Behavior", finalLogMessage)
}

func getDrivingBehaviorNotificationsByUserId(userId int, lastDateTime string) ([]DrivingBehavior, error) {
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
	// Defind the request body
	requestBody := map[string]interface{}{
		"userId":          userId,
		"LastdateTime":    lastDateTime,
		"catagoryGroupId": 7102,
	}
	bodyBytes, err := json.Marshal(requestBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request body: %v", err)
	}

	// Define the HTTP request
	req, err := http.NewRequest("POST", fmt.Sprintf("%s/notifications/vehicle-ai-mdvr-notification", apiURL), bytes.NewBuffer(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request: %v", err)
	}

	// Set the request headers
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
				return nil, fmt.Errorf("error sending to Driving Behavior Notifications after %d retries: %w", maxRetries, err)
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
		var apiResponse DrivingAPIResponse
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

func formatDrivingBehaviorNotifications(notifications []DrivingBehavior, URL string) []string {
	var messages []string

	for _, notification := range notifications {
		lastTime, exists := lastProcessedTimeDriving[notification.VehicleID]

		// ตรวจสอบว่าการแจ้งเตือนนี้เป็นรายการเดิมหรือไม่
		if exists && notification.GPSDateTime == lastTime {
			continue
		}

		// อัปเดตเวลาล่าสุดของ VehicleID นั้นๆ
		lastProcessedTimeDriving[notification.VehicleID] = notification.GPSDateTime
		message := fmt.Sprintf(
			"แจ้งเตือน %s\n"+
				"เลขทะเบียน: %s\n"+
				"วันเวลา: %s\n"+
				"ความเร็ว: %d กม./ซม.\n"+
				"สถานที่: %s\n"+
				"https://www.google.com/maps?q=%s,%s\n"+
				"วีดิโอ: %s",
			notification.EventNameTH,
			notification.LicenseNo,
			notification.GPSDateTime,
			notification.Speed,
			notification.LocationTH,
			notification.Latitude,
			notification.Longitude,
			URL)

		messages = append(messages, message)
	}

	return messages
}
