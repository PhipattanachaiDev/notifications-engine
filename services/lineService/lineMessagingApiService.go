package lineService

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
)

// sendToLineGroup ส่งข้อความไปยัง LINE Group โดยรับ token, groupID และข้อความเป็นพารามิเตอร์
func SendToLineGroup(channelAccessToken, groupID, message string) error {
	if channelAccessToken == "" {
		return fmt.Errorf("LINE Channel Access Token is required")
	}
	if groupID == "" {
		return fmt.Errorf("LINE Group ID is required")
	}
	if message == "" {
		return fmt.Errorf("message cannot be empty")
	}

	url := "https://api.line.me/v2/bot/message/push"

	// ข้อมูล payload ที่จะส่ง
	payload := map[string]interface{}{
		"to": groupID,
		"messages": []map[string]string{
			{
				"type": "text",
				"text": message,
			},
		},
	}

	// แปลง payload เป็น JSON
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("error marshalling JSON: %w", err)
	}

	// สร้าง HTTP request
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(body))
	if err != nil {
		return fmt.Errorf("error creating request: %w", err)
	}

	// ตั้งค่า Header
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+channelAccessToken)

	// ส่ง request
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("error sending to LINE Group: %w", err)
	}
	defer resp.Body.Close()

	// ตรวจสอบ response
	if resp.StatusCode != http.StatusOK {
		// อ่าน response body เมื่อเกิดข้อผิดพลาด
		var errorResponse map[string]interface{}
		_ = json.NewDecoder(resp.Body).Decode(&errorResponse)
		return fmt.Errorf("failed to send message, status: %s, error: %v", resp.Status, errorResponse)
	}

	fmt.Println("Message sent to LINE Group successfully")
	return nil
}
