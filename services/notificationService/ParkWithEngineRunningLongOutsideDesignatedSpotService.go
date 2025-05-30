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

// จอดติดเครื่องนานนอกจุดจอดที่กำหนด
type LongIdleOutsideSpotAPIResponse struct {
	Success   bool                                   `json:"success"`
	Status    int                                    `json:"status"`
	Timestamp string                                 `json:"timestamp"`
	Message   string                                 `json:"message"`
	Data      []ParkWithEngineRunningLongOutsideSpot `json:"data"`
}

type ParkWithEngineRunningLongOutsideSpot struct {
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

func ParkWithEngineRunningLongOutsideDesignatedSpot(startTime time.Time, settings map[string]interface{}, userNotifySetting *[]userService.UserNotifySetting) {
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

		ParkWithEngineRunningLongOutsideDesignatedSpotNotifications, err := getParkWithEngineRunningLongOutsideDesignatedSpotNotificationsByUserId(user.UserID)
		if err != nil {
			logService.WriteLog("park_with_engine_running_long_outside_designated_spot_error", fmt.Sprintf("failed to fetch park with engine running long outside designated spot notifications for user %d: %v", user.UserID, err))
			continue
		}
		
		// ตรวจสอบว่ามีการแจ้งเตือนใน parkWithEngineRunningLongOutsideDesignatedSpotNotifications หรือไม่
		if len(ParkWithEngineRunningLongOutsideDesignatedSpotNotifications) > 0 {
			// อัพเดท LastMessageGpsDateTime ด้วยเวลาของการแจ้งเตือนล่าสุด
			latestNotification := ParkWithEngineRunningLongOutsideDesignatedSpotNotifications[len(ParkWithEngineRunningLongOutsideDesignatedSpotNotifications)-1]
			// Correct time format for parsing the input
			layout := "02/01/2006 15:04:05"

			parsedTime, err := time.Parse(layout, latestNotification.GPSDateTime)
			if err != nil {
				logService.WriteLog("park_with_engine_running_long_outside_designated_spot_error", fmt.Sprintf("failed to parse time: %v", err))
				continue
			}
			(*userNotifySetting)[i].LastMessageGPSDateTime = parsedTime
		}

		messages := formatParkWithEngineRunningLongOutsideDesignatedSpot(ParkWithEngineRunningLongOutsideDesignatedSpotNotifications)

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

		err = pEngineRunningLongOutsideDesignatedSpotNotificationsLogs(user.UserID, ParkWithEngineRunningLongOutsideDesignatedSpotNotifications)
		if err != nil {
			logService.WriteLog("park_with_engine_running_long_outside_designated_spot_error", fmt.Sprintf("failed to insert notifications log for user %d: %v", user.UserID, err))
			continue
		}
	}

	// บันทึกจำนวนข้อความที่ส่งและข้อผิดพลาดใน log
	finalLogMessage := fmt.Sprintf("Total notifications sent: %d, Total errors: %d", totalNotificationsSent, totalErrors)
	logService.WriteLog("park_with_engine_running_long_outside_designated_spot", finalLogMessage)
}

func formatParkWithEngineRunningLongOutsideDesignatedSpot(notifications []ParkWithEngineRunningLongOutsideSpot) []string {
	var messages []string
	startTime := time.Now().UTC()

	// แปลงเวลา UTC เป็น UTC+7
	location, err := time.LoadLocation("Asia/Bangkok")
	if err != nil {
		// logService.WriteLog(inst.ID, fmt.Sprintf("Error loading location: %v", err))
		location = time.FixedZone("Asia/Bangkok", 7*60*60) // ใช้ UTC+7 แบบคงที่
	}

	utcPlus7 := startTime.In(location).Format("02/01/2006 15:04:05")
	for _, notification := range notifications {
		message := fmt.Sprintf(
			"แจ้งเตือนจอดติดเครื่องนานเกินกำหนดนอกจุดจอดในสถานที่กำหนด (POI)\n"+
				"เลขทะเบียน: %s\n"+
				"วันเวลาจอดติดเครื่อง: %s\n"+
				"วันเวลาปัจจุบัน: %s\n"+
				"สถานที่: %s\n"+
				"https://www.google.com/maps?q=%s,%s", notification.LicenseNo, notification.EngineOnDateTime, utcPlus7, notification.LocationTH, notification.Latitude, notification.Longitude)
		messages = append(messages, message)
	}

	return messages
}

func getParkWithEngineRunningLongOutsideDesignatedSpotNotificationsByUserId(userId int) ([]ParkWithEngineRunningLongOutsideSpot, error) {
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
		"userId":        userId,
		"eventId":       6007,
		"inoutWaypoint": 1,
		"isEngineOn":    true,
	}
	bodyBytes, err := json.Marshal(requestBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request body: %v", err)
	}
	// Create the HTTP request
	req, err := http.NewRequest("POST", fmt.Sprintf("%s/notifications/vehicle-parking-notification", apiURL), bytes.NewBuffer(bodyBytes))
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
				return nil, fmt.Errorf("error sending to park With Engine Running Long Outside Designated Spot Notifications after %d retries: %w", maxRetries, err)
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
		var apiResponse LongIdleOutsideSpotAPIResponse
		if err := json.NewDecoder(resp.Body).Decode(&apiResponse); err != nil {
			return nil, fmt.Errorf("failed to decode JSON response: %v", err)
		}

		// Check if the API response indicates success
		if !apiResponse.Success {
			return nil, fmt.Errorf("API response indicates failure: %s", apiResponse.Message)
		}

		return apiResponse.Data, nil
	}

	return nil, fmt.Errorf("failed to get data from API after %d retries", maxRetries)

}

func pEngineRunningLongOutsideDesignatedSpotNotificationsLogs(userId int, PEngingrunninglongspot []ParkWithEngineRunningLongOutsideSpot) error {
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

	for _, parked := range PEngingrunninglongspot {
		t, _ := time.Parse("02/01/2006 15:04:05", parked.GPSDateTime)
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
			"vehicleId":            parked.VehicleID,
			"eventId":              6007,
			"gpsDate":              gpsDate,
			"gpsTime":              gpsTime,
			"latitude":             parked.Latitude,
			"longitude":            parked.Longitude,
			"locationTH":           parked.LocationTH,
			"waypointLocationId":   0,
			"waypointLocationName": parked.WaypointLocationNameTH,
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
