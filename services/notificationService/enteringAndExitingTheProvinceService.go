package notificationServices

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"time"

	"ezview.com/engine/notifications/services/lineService"
	"ezview.com/engine/notifications/services/logService"
	telegramService "ezview.com/engine/notifications/services/telegramService"
	userService "ezview.com/engine/notifications/services/userService"
	"github.com/joho/godotenv"
)

type EtrAndExtTheProvinceAPIResonse struct {
	Success   bool                   `json:"success"`
	Status    int                    `json:"status"`
	Timestamp string                 `json:"timestamp"`
	Message   string                 `json:"message"`
	Data      []EtrAndExtTheProvince `json:"data"`
}

type EtrAndExtTheProvince struct {
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

func EnteringAndExitingTheProvinceNotification(startTime time.Time, settings map[string]interface{}, userNotifySetting *[]userService.UserNotifySetting) {
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

		// Fetch EnteringAndExitingTheProvince notifications for the user
		EtrAndExitTheProvinceNotifications, err := getEnteringAndExitingTheProvinceNotificationsByUserId(user.UserID)
		if err != nil {
			logService.WriteLog("entering_and_exitting_the_province_error", fmt.Sprintf("failed to get entering and exiting the province notifications for user %d: %v", user.UserID, err))
			continue
		}

		// ตรวจสอบว่ามีการแจ้งเตือนใน EnteringAndExitingTheProvince หรือไม่
		if len(EtrAndExitTheProvinceNotifications) > 0 {
			// อัพเดท LastMessageGPSDateTime ด้วยเวลาของการแจ้งเตือนล่าสุด
			latestNotification := EtrAndExitTheProvinceNotifications[len(EtrAndExitTheProvinceNotifications)-1]
			// Correct time format for parsing the input
			layout := "02/01/2006 15:04:05"

			// Try parsing the time with the correct layout
			parsedTime, err := time.Parse(layout, latestNotification.GPSDateTime)
			if err != nil {
				logService.WriteLog("entering_and_exitting_the_province_error", fmt.Sprintf("failed to parse time: %v", err))
			}
			(*userNotifySetting)[i].LastMessageGPSDateTime = parsedTime
		}

		// Format the notifications
		messages := formatEnteringAndExitingTheProvincenotifications(EtrAndExitTheProvinceNotifications)

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

		err = EnteringAndExitingTheProvinceLogs(user.UserID, EtrAndExitTheProvinceNotifications)
		if err != nil {
			logService.WriteLog("entering_and_exitting_the_province_error", fmt.Sprintf("failed to insert notifications log for user %d: %v", user.UserID, err))
			continue
		}
	}

	// บันทึกจำนวนข้อความที่ส่งและข้อผิดพลาดใน log
	finalLogMessage := fmt.Sprintf("Total notifications sent: %d, Total errors: %d", totalNotificationsSent, totalErrors)
	logService.WriteLog("entering_and_exitting_the_province", finalLogMessage)
}

func getEnteringAndExitingTheProvinceNotificationsByUserId(userId int) ([]EtrAndExtTheProvince, error) {
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
		"userId":  userId,
		"eventId": 6021,
	}
	bodyBytes, err := json.Marshal(requestBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request body: %v", err)
	}

	// Define the HTTP request
	req, err := http.NewRequest("POST", fmt.Sprintf("%s/notifications/vehicle-in-out-province-notification", apiURL), bytes.NewBuffer(bodyBytes))
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
				return nil, fmt.Errorf("error sending to Entering And Exitting The Province Notifications after %d retries: %w", maxRetries, err)	
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
		var apiResponse EtrAndExtTheProvinceAPIResonse
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

func formatEnteringAndExitingTheProvincenotifications(notifications []EtrAndExtTheProvince) []string {
	var messages []string

	for _, notification := range notifications {
		var message string
		if notification.InOutWaypointStatus == 1 {
			message = fmt.Sprintf(
				"แจ้งเตือน เข้าเขต %s\n"+
					"เลขทะเบียน: %s\n"+
					"วันเวลา: %s\n"+
					"สถานที่: %s\n"+
					"https://www.google.com/maps?q=%s,%s",
				notification.PoiName,
				notification.LicenseNo,
				notification.GPSDateTime,
				notification.LocationTH,
				notification.Latitude,
				notification.Longitude)
		} else {
			message = fmt.Sprintf(
				"แจ้งเตือน ออกเขต %s\n"+
					"เลขทะเบียน: %s\n"+
					"วันเวลา: %s\n"+
					"สถานที่: %s\n"+
					"https://www.google.com/maps?q=%s,%s",
				notification.PoiName,
				notification.LicenseNo,
				notification.GPSDateTime,
				notification.LocationTH,
				notification.Latitude,
				notification.Longitude)
		}
		messages = append(messages, message)
	}
	return messages
}

func EnteringAndExitingTheProvinceLogs(userId int, EtrAndExtTheProvince []EtrAndExtTheProvince) error {
	// Load .env file
	err := godotenv.Load()
	if err != nil {
		return fmt.Errorf("failed to load .env file: %v", err)
	}

	// Get API URL from .env
	apiURL := os.Getenv("API_URL")
	if apiURL == "" {
		return fmt.Errorf("API_URL is not set in .env")
	}

	// Get Authorization token from .env
	authToken := os.Getenv("JWT_TOKEN")
	if authToken == "" {
		return fmt.Errorf("JWT_TOKEN is not set in .env")
	}

	for _, eTrAndExt := range EtrAndExtTheProvince {

		t, _ := time.Parse("02/01/2006 15:04:05", eTrAndExt.GPSDateTime)
		gpsDateStr := t.Format("20060102")
		gpsTimeStr := t.Format("150405")

		gpsDate, _ := strconv.Atoi(gpsDateStr)
		gpsTime, _ := strconv.Atoi(gpsTimeStr)

		startTime := time.Now().UTC()
		location, err := time.LoadLocation("Asia/Bangkok")
		if err != nil {
			location = time.FixedZone("Asia/Bangkok", 7*60*60) // ใช้ UTC+7 แบบคงที่
		}

		utcPlus7 := startTime.In(location)
		lastNotiDateStr := utcPlus7.Format("20060102")
		lastNotiTimeStr := utcPlus7.Format("150405")

		lastNotiDate, _ := strconv.Atoi(lastNotiDateStr)
		lastNotiTime, _ := strconv.Atoi(lastNotiTimeStr)

		requestBody := map[string]interface{}{
			"userId":               userId,
			"vehicleId":            eTrAndExt.VehicleID,
			"eventId":              6021,
			"gpsDate":              gpsDate,
			"gpsTime":              gpsTime,
			"latitude":             eTrAndExt.Latitude,
			"longitude":            eTrAndExt.Longitude,
			"locationTH":           eTrAndExt.LocationTH,
			"waypointLocationId":   0,
			"waypointLocationName": eTrAndExt.WaypointLocationNameTH,
			"lastNotificationDate": lastNotiDate,
			"lastNotificationTime": lastNotiTime,
		}
		// Defind the request body
		bodyBytes, err := json.Marshal(requestBody)
		if err != nil {
			return fmt.Errorf("failed to marshal request body: %v", err)
		}

		// Define the HTTP request
		req, err := http.NewRequest("POST", fmt.Sprintf("%s/notifications/vehicle-notification/log", apiURL), bytes.NewBuffer(bodyBytes))
		if err != nil {
			return fmt.Errorf("failed to create HTTP request: %v", err)
		}
		// Set the request headers
		req.Header.Set("accept", "application/json")
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", authToken))

		var resp *http.Response
		// Make the HTTP request
		client := &http.Client{}
		resp, err = client.Do(req)
		if err != nil {
			return fmt.Errorf("failed to make HTTP request: %v", err)
		}
		defer resp.Body.Close()

		// อ่าน response body ออกมาเป็น string
		respBody, err := io.ReadAll(resp.Body)
		if err != nil {
			return fmt.Errorf("failed to read response body: %v", err)
		}

		// Check HTTP status
		if resp.StatusCode != http.StatusCreated {
			return fmt.Errorf("failed to get data from API, status code: %d, response: %s", resp.StatusCode, string(respBody))
		}
	}
	return nil
}
