package notificationServices

import (
	"bytes"
	"encoding/json"
	"ezview.com/engine/notifications/services/lineService"
	"ezview.com/engine/notifications/services/logService"
	telegramService "ezview.com/engine/notifications/services/telegramService"
	userService "ezview.com/engine/notifications/services/userService"
	"fmt"
	"github.com/joho/godotenv"
	"net/http"
	"os"
	"time"
)

type EtrAPIResponse struct {
	Success   bool               `json:"success"`
	Status    int                `json:"status"`
	Timestamp string             `json:"timestamp"`
	Message   string             `json:"message"`
	Data      []EtrNoparkingArea `json:"data"`
}

type EtrNoparkingArea struct {
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

func EnteringNoparkingArea(startTime time.Time, settings map[string]interface{}, userNotifySetting *[]userService.UserNotifySetting) {
	var totalNotificationsSent int
	var totalErrors int

	for i, user := range *userNotifySetting {
		var lastDateTime time.Time

		if user.LastMessageGPSDateTime.IsZero() {
			lastDateTime = startTime.Add(-5 * time.Minute)
		} else {
			lastDateTime = user.LastMessageGPSDateTime
		}

		enteringNoparkingArea, err := getEtrNoparkingAreaNotificationsById(user.UserID, lastDateTime.Format(time.RFC3339))
		if err != nil {
			logService.WriteLog("etr_noparking_area_error", fmt.Sprintf("Failed to fetch notifications for UserID %d: %v", user.UserID, err))
			continue
		}

		// ตรวจสอบว่ามีการแจ้งเตือนใน enteringNoparkingArea หรือไม่
		if len(enteringNoparkingArea) > 0 {
			// อัปเดต `LastMessageGPSDateTime` ด้วยเวลาของการแจ้งเตือนล่าสุด
			latestNotification := enteringNoparkingArea[len(enteringNoparkingArea)-1]
			// Correct time format for parsing the input
			layout := "02/01/2006 15:04:05"

			//try parsing the time with the correct layout
			parsedTime, err := time.Parse(layout, latestNotification.GPSDateTime)
			if err != nil {
				logService.WriteLog("etr_noparking_area_error", fmt.Sprintf("Failed to parse GPSDateTime for UserID %d: %v", user.UserID, err))
				continue
			}
			(*userNotifySetting)[i].LastMessageGPSDateTime = parsedTime
		}

		// จัดรูปแบบข้อความการแจ้งเตือน
		messages := etrformatNotifications(enteringNoparkingArea)

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
	logService.WriteLog("etr_noparking_area", finalLogMessage)
}

func getEtrNoparkingAreaNotificationsById(userId int, lastDateTime string) ([]EtrNoparkingArea, error) {
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
		"LastDateTime":         lastDateTime,
		"UserID":               userId,
		"messageType":          []string{"IRW"},
		"eventId":              6002,
		"deleteLogMessageType": []string{"ORW"},
	}
	bodyBytes, err := json.Marshal(requestBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request body: %v", err)
	}

	// Create the HTTP request
	req, err := http.NewRequest("POST", fmt.Sprintf("%s/notifications/vehicle-entering-noparking-area-notification", apiURL), bytes.NewBuffer(bodyBytes))
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
				return nil, fmt.Errorf("error sending to Entering No Parking Area Notifications after %d retries: %w", maxRetries, err)	
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
		var apiResponse EtrAPIResponse
		if err := json.NewDecoder(resp.Body).Decode(&apiResponse); err != nil {
			return nil, fmt.Errorf("failed to decode JSON response: %v", err)
		}
	
		// check if the API response indicates success
		if !apiResponse.Success {
			return nil, fmt.Errorf("API response error: %s", apiResponse.Message)
		}
	
		return apiResponse.Data, nil
	}

	return nil, fmt.Errorf("failed to get data from API after %d retries", maxRetries)
	

}

func etrformatNotifications(notifications []EtrNoparkingArea) []string {
	var messages []string

	for _, area := range notifications {
		message := fmt.Sprintf(
			"แจ้งเตือนเข้าจุดห้ามจอด\n"+
				"บริษัท: %s\n"+
				"เลขทะเบียน: %s\n"+
				"วันเวลา: %s\n"+
				"จุดจอด: %s\n"+
				"สถานที่: %s\n"+
				"https://www.google.com/maps?q=%s,%s", area.CustomerName, area.LicenseNo, area.GPSDateTime, area.WaypointLocationNameTH, area.LocationTH, area.Latitude, area.Longitude)
		messages = append(messages, message)
	}

	return messages
}
