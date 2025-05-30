package configService

import (
	"encoding/json"
	"fmt"
	"os"
)

func CreateDefaultSettings(id string) error {
	// ค่า settings เริ่มต้น
	defaultSettings := map[string]interface{}{
		"auto_start": false,
		"interval":   60, // ค่า interval เริ่มต้น (วินาที)
	}

	// สร้าง path สำหรับไฟล์ settings
	configDir := "./configs"
	if err := os.MkdirAll(configDir, os.ModePerm); err != nil {
		return fmt.Errorf("failed to create configs directory: %v", err)
	}

	filePath := fmt.Sprintf("%s/%s_settings.json", configDir, id)
	file, err := os.Create(filePath)
	if err != nil {
		return fmt.Errorf("failed to create settings file: %v", err)
	}
	defer file.Close()

	// เขียนค่า default settings ลงไฟล์ JSON
	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ") // ตั้งค่าให้ JSON สวยงาม
	if err := encoder.Encode(defaultSettings); err != nil {
		return fmt.Errorf("failed to write default settings: %v", err)
	}

	fmt.Printf("Default settings created for instance %s: %s\n", id, filePath)
	return nil
}
