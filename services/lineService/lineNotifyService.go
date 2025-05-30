package lineService

import (
	"bytes"
	"fmt"
	"io"
	"net/http"

	"ezview.com/engine/notifications/services/logService"
)

type LineSetting struct {
	// AccessToken ของ LINE Notify ที่จะใช้ในการส่งข้อความ
	AccessToken string `json:"lineSetting"`
}

// SendToLineNotify ส่งข้อความไปยัง LINE Notify โดยรับ token และ message เป็นพารามิเตอร์
func SendToLineNotify(token, message string) error {
	if token == "" {
		return fmt.Errorf("LINE Notify token is required")
	}
	if message == "" {
		return fmt.Errorf("message cannot be empty")
	}

	url := "https://notify-api.line.me/api/notify"
	data := "message=" + message

	req, err := http.NewRequest("POST", url, bytes.NewBufferString(data))
	if err != nil {
		return fmt.Errorf("error creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", "Bearer "+token)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("error sending request: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to send message, status: %s, body: %s", resp.Status, body)
	}

	// เพิ่มข้อความการส่งสำเร็จ
	fmt.Println("Message sent to LINE Notify successfully")
	return nil
}

// ฟังก์ชันส่งข้อความไปยัง Line
func SendToLine(lineSettings []LineSetting, messages []string) (int, int) {
	totalErrors := 0
	totalMessagesSent := 0

	// ลูปส่งข้อความไปยังแต่ละ Line AccessToken
	for _, setting := range lineSettings {
		for _, message := range messages {
			// พยายามส่งข้อความ
			err := SendToLineNotify(setting.AccessToken, message)
			if err != nil {
				// หากเกิดข้อผิดพลาด เพิ่มตัวนับข้อผิดพลาด
				logService.WriteLog("line_error", fmt.Sprintf("Failed to send message to Line AccessToken %s: %v", setting.AccessToken, err))
				totalErrors++
			} else {
				// นับจำนวนห้องที่ส่งข้อความสำเร็จ
				totalMessagesSent++
			}
		}
	}
	return totalMessagesSent, totalErrors
}
