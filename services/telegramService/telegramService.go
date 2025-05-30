package telegramService

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"ezview.com/engine/notifications/services/logService"
)

type TelegramSetting struct {
	BotID   string   `json:"botId"`
	ChatIDs []string `json:"chatIds"`
}

// sendToTelegram ส่งข้อความไปยัง Telegram โดยรับ token, chatID และข้อความเป็นพารามิเตอร์
func SendMessageTelegram(token, chatID, message string) error {

	if token == "" {
		return fmt.Errorf("telegram bot token is required")
	}
	if chatID == "" {
		return fmt.Errorf("telegram chat ID is required")
	}
	if message == "" {
		return fmt.Errorf("message cannot be empty")
	}

	url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", token)

	payload := map[string]string{
		"chat_id": chatID,
		"text":    message,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("error marshaling payload: %w", err)
	}

	maxRetries := 5
	for i := 0; i < maxRetries; i++ {
		resp, err := http.Post(url, "application/json", bytes.NewBuffer(body))
		if err != nil {
			if i == maxRetries-1 {
				return fmt.Errorf("error sending to Telegram after %d retries: %w", maxRetries, err)
			}
			time.Sleep(time.Second * time.Duration(i+1)) // หน่วงเวลาเพิ่มขึ้นตามรอบที่พยายาม
			continue
		}

		defer resp.Body.Close()

		if resp.StatusCode == http.StatusOK {
			return nil // ส่งสำเร็จ
		}

		if resp.StatusCode == 429 {
			// อ่านค่า Retry-After header และรอเวลาที่กำหนด
			retryAfter := resp.Header.Get("Retry-After")
			if retryAfter != "" {
				delay, _ := strconv.Atoi(retryAfter)
				time.Sleep(time.Duration(delay) * time.Second)
			}
		} else {
			if i == maxRetries-1 {
				return fmt.Errorf("failed to send message to Telegram, status: %s", resp.Status)
			}
			time.Sleep(time.Second * time.Duration(i+1))
		}
	}

	return fmt.Errorf("failed to send message to Telegram after %d retries", maxRetries)
}

// ฟังก์ชันส่งข้อความไปยัง Telegram
func SendToTelegram(telegramSettings []TelegramSetting, messages []string) (int, int) {
	totalErrors := 0
	totalMessagesSent := 0

	for _, setting := range telegramSettings {
		for _, chatID := range setting.ChatIDs {
			
			for _, message := range messages {
				// พยายามส่งข้อความ
				
				err := SendMessageTelegram(setting.BotID, chatID, message)
				if err != nil {
					// หากเกิดข้อผิดพลาด เพิ่มตัวนับข้อผิดพลาด
					logService.WriteLog("telegram_error", fmt.Sprintf("Failed to send message to BotID %s, ChatID %s: %v", setting.BotID, chatID, err))
					totalErrors++
				} else {
					// นับจำนวนห้องที่ส่งข้อความสำเร็จ
					totalMessagesSent++
				}
			}
		}
	}
	return totalMessagesSent, totalErrors
}
