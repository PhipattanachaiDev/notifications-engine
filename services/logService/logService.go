package logService

import (
	"fmt"
	"log"
	"os"
	"time"
)

// WriteLog ฟังก์ชันสำหรับเขียน log ลงไฟล์ในแต่ละ instance โดยแยกไฟล์ตามวันที่
func WriteLog(instanceID, message string) {
	// ใช้วันที่ปัจจุบันเป็นชื่อไฟล์ log
	today := time.Now().Format("2006-01-02")        // format เป็น yyyy-mm-dd
	logFolder := fmt.Sprintf("logs/%s", instanceID) // โฟลเดอร์ที่เกี่ยวข้องกับ instance
	logFileName := fmt.Sprintf("%s/%s_log_%s.log", logFolder, instanceID, today)

	// สร้างโฟลเดอร์สำหรับ instance หากยังไม่มี
	if err := os.MkdirAll(logFolder, 0755); err != nil {
		fmt.Printf("Error creating logs directory for instance %s: %v\n", instanceID, err)
		return
	}

	// เปิดหรือสร้างไฟล์ log
	logFile, err := os.OpenFile(logFileName, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		fmt.Printf("Error opening log file for instance %s: %v\n", instanceID, err)
		return
	}
	defer logFile.Close()

	// สร้าง logger และเขียนข้อความลงในไฟล์ log
	logger := log.New(logFile, "", log.LstdFlags)
	logger.Println(message)
}
