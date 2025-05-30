package controllers

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"ezview.com/engine/notifications/services/logService"
	"ezview.com/engine/notifications/services/userService"
)

// Instance struct สำหรับจัดการสถานะและงานเฉพาะของแต่ละ instance
type Instance struct {
	ID        string
	EventId   int
	IsRunning bool
	Settings  map[string]interface{} // เก็บค่าการตั้งค่าแบบไดนามิก
	Users     []userService.UserNotifySetting
	Ticker    *time.Ticker
	Done      chan bool
	Task      func(startTime time.Time, settings map[string]interface{}, userNotifySetting *[]userService.UserNotifySetting) // ฟังก์ชันสำหรับการทำงานเฉพาะของ instance
}

// Controller struct สำหรับเก็บ mutex และ instances
type Controller struct {
	Mutex     *sync.Mutex
	Instances map[string]*Instance
}

func (c *Controller) StartHandler(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if id == "" {
		http.Error(w, "Missing 'id' parameter", http.StatusBadRequest)
		return
	}

	c.Mutex.Lock()
	defer c.Mutex.Unlock()

	instance, exists := c.Instances[id]
	if !exists {
		http.Error(w, "Instance not found", http.StatusNotFound)
		return
	}

	if instance.IsRunning {
		fmt.Fprintf(w, "Instance %s is already running\n", id)
		return
	}

	// อ่านการตั้งค่าจากไฟล์
	settings, err := ReadSettings(id)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to read settings file: %v", err), http.StatusInternalServerError)
		logService.WriteLog(id, fmt.Sprintf("Error reading settings file: %v", err))
		return
	}

	instance.Settings = settings
	interval, ok := settings["interval"].(float64)
	if !ok || interval <= 0 {
		http.Error(w, "Invalid or missing 'interval' in settings", http.StatusBadRequest)
		logService.WriteLog(id, "Invalid or missing 'interval' in settings")
		return
	}

	// เริ่ม Instance
	instance.IsRunning = true
	instance.Done = make(chan bool)
	instance.Ticker = time.NewTicker(time.Duration(interval) * time.Second)

	// ดึงผู้ใช้สำหรับ Instance
	users, err := userService.GetUsersNotifySetting(instance.EventId)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to fetch users for instance %s: %v", id, err), http.StatusInternalServerError)
		logService.WriteLog(instance.ID+"_error", fmt.Sprintf("Failed to fetch users: %v", err))
		users = []userService.UserNotifySetting{} // ใช้ array ว่างเป็นค่าเริ่มต้นแทน
	}

	instance.Users = users

	go func(inst *Instance) {
		// เริ่มต้น startTime ที่ได้รับจากภายนอก
		startTime := time.Now().UTC()

		for {
			select {
			case <-inst.Ticker.C:
				// เพิ่มเวลาทุกๆ รอบ
				startTime = startTime.Add(1 * time.Minute) // เพิ่มเวลา 1 นาทีทุกครั้ง

				// แปลงเวลา UTC เป็น UTC+7
				location, err := time.LoadLocation("Asia/Bangkok")
				if err != nil {
					// logService.WriteLog(inst.ID, fmt.Sprintf("Error loading location: %v", err))
					location = time.FixedZone("Asia/Bangkok", 7*60*60) // ใช้ UTC+7 แบบคงที่
				}

				utcPlus7 := startTime.In(location)

				// ดึงข้อมูลผู้ใช้ใหม่ทุกครั้ง
				users, err := userService.GetUsersNotifySetting(instance.EventId)
				if err != nil {
					logService.WriteLog(inst.ID+"_error", fmt.Sprintf("Error fetching updated users: %v", err))
					continue // ข้ามรอบนี้และดำเนินการในรอบถัดไป
				}

				// อัปเดตผู้ใช้ใน instance
				inst.Users = userService.UpdateUsers(inst.Users, users)

				// ส่งเวลา UTC+7 ที่อัปเดตไปให้ Task
				inst.Task(utcPlus7, settings, &inst.Users)

			case <-inst.Done:
				inst.Ticker.Stop()
				fmt.Printf("Instance %s: Stopped ticker\n", inst.ID)
				return
			}
		}
	}(instance)

	fmt.Fprintf(w, "Instance %s started with interval %d seconds\n", id, int(interval))
}

// StopHandler สำหรับหยุด instance
func (c *Controller) StopHandler(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if id == "" {
		http.Error(w, "Missing 'id' parameter", http.StatusBadRequest)
		return
	}

	c.Mutex.Lock()
	defer c.Mutex.Unlock()

	instance, exists := c.Instances[id]
	if !exists {
		http.Error(w, "Instance not found", http.StatusNotFound)
		return
	}

	if !instance.IsRunning {
		fmt.Fprintf(w, "Instance %s is not running\n", id)
		return
	}

	instance.IsRunning = false
	close(instance.Done)
	fmt.Fprintf(w, "Instance %s stopped\n", id)
}

// StatusHandler สำหรับดูสถานะ instance เดียว
func (c *Controller) StatusHandler(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if id == "" {
		http.Error(w, "Missing 'id' parameter", http.StatusBadRequest)
		return
	}

	c.Mutex.Lock()
	defer c.Mutex.Unlock()

	instance, exists := c.Instances[id]
	if !exists {
		http.Error(w, "Instance not found", http.StatusNotFound)
		return
	}

	status := map[string]interface{}{
		"id":         instance.ID,
		"is_running": instance.IsRunning,
		"settings":   instance.Settings,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}

// StatusAllHandler สำหรับดูสถานะทุก instance
func (c *Controller) StatusAllHandler(w http.ResponseWriter, r *http.Request) {
	c.Mutex.Lock()
	defer c.Mutex.Unlock()

	var statuses []map[string]interface{}
	for id, instance := range c.Instances {
		statuses = append(statuses, map[string]interface{}{
			"id":         id,
			"is_running": instance.IsRunning,
			"settings":   instance.Settings,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(statuses)
}

// SetSettingsHandler สำหรับอัปเดตการตั้งค่า
func (c *Controller) SetSettingsHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID       string                 `json:"id"`
		Settings map[string]interface{} `json:"settings"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON payload", http.StatusBadRequest)
		return
	}

	if req.ID == "" {
		http.Error(w, "Missing 'id' in request payload", http.StatusBadRequest)
		return
	}

	c.Mutex.Lock()
	defer c.Mutex.Unlock()

	instance, exists := c.Instances[req.ID]
	if !exists {
		http.Error(w, "Instance not found", http.StatusNotFound)
		return
	}

	// ตรวจสอบว่า instance กำลังทำงานอยู่
	if instance.IsRunning {
		// แจ้งผู้ใช้ว่า instance กำลังทำงานอยู่ และไม่ให้ทำการอัปเดต
		fmt.Fprintf(w, "Instance %s is currently running. Settings cannot be updated until it stops.\n", req.ID)
	} else {
		// อัปเดตการตั้งค่า
		for key, value := range req.Settings {
			instance.Settings[key] = value
		}

		// เขียนการตั้งค่าใหม่ไปที่ไฟล์
		if err := WriteSettings(req.ID, instance.Settings); err != nil {
			http.Error(w, fmt.Sprintf("Failed to write settings file: %v", err), http.StatusInternalServerError)
			return
		}

		fmt.Fprintf(w, "Settings for instance %s updated\n", req.ID)
	}
}

// ReadSettings อ่านการตั้งค่าจากไฟล์
func ReadSettings(id string) (map[string]interface{}, error) {
	filePath := filepath.Join("configs", fmt.Sprintf("%s_settings.json", id))
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return map[string]interface{}{"interval": 10}, nil
	}

	data, err := ioutil.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	var settings map[string]interface{}
	if err := json.Unmarshal(data, &settings); err != nil {
		return nil, err
	}

	return settings, nil
}

// WriteSettings เขียนการตั้งค่าลงไฟล์
func WriteSettings(id string, settings map[string]interface{}) error {
	filePath := filepath.Join("configs", fmt.Sprintf("%s_settings.json", id))
	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}

	if err := os.MkdirAll("configs", 0755); err != nil {
		return err
	}

	return ioutil.WriteFile(filePath, data, 0644)
}

// GetLogHandler สำหรับดึงข้อมูล log ของ instance ในช่วงวันที่ที่กำหนด พร้อมกับจำกัดจำนวนบรรทัดที่ดึงมา
func (c *Controller) GetLogHandler(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if id == "" {
		http.Error(w, "Missing 'id' parameter", http.StatusBadRequest)
		return
	}

	// รับ query parameters สำหรับช่วงวันที่ (start_date, end_date)
	startDate := r.URL.Query().Get("start_date")
	endDate := r.URL.Query().Get("end_date")
	limitParam := r.URL.Query().Get("limit")
	sortOrder := r.URL.Query().Get("sort_order") // ค่าของ sort_order เช่น "asc" หรือ "desc"

	// ถ้าไม่ได้ระบุวันที่เริ่มต้นและวันที่สิ้นสุด ให้ใช้วันที่ปัจจุบัน
	if startDate == "" {
		startDate = time.Now().Format("2006-01-02")
	}
	if endDate == "" {
		endDate = time.Now().Format("2006-01-02")
	}

	// แปลงวันที่ที่ได้รับเป็น time.Time
	start, err := time.Parse("2006-01-02", startDate)
	if err != nil {
		http.Error(w, fmt.Sprintf("Invalid start_date format: %v", err), http.StatusBadRequest)
		return
	}

	end, err := time.Parse("2006-01-02", endDate)
	if err != nil {
		http.Error(w, fmt.Sprintf("Invalid end_date format: %v", err), http.StatusBadRequest)
		return
	}

	// ตั้งค่า default limit ถ้าไม่มีการระบุ
	limit := -1 // ถ้า limit เป็น -1 หมายถึงไม่จำกัดจำนวนบรรทัด
	if limitParam != "" {
		limit, err = strconv.Atoi(limitParam)
		if err != nil || limit <= 0 {
			http.Error(w, "Invalid 'limit' value", http.StatusBadRequest)
			return
		}
	}

	// กำหนด path ของไฟล์ log ตามวันที่
	var logData []string
	logCount := 0
	for current := start; !current.After(end); current = current.Add(24 * time.Hour) {
		// คำนวณ path ของไฟล์ log สำหรับแต่ละวัน
		logFilePath := filepath.Join("logs", id, fmt.Sprintf("%s_log_%s.log", id, current.Format("2006-01-02")))

		// เปิดไฟล์ log
		file, err := os.Open(logFilePath)
		if err != nil {
			// ถ้าไฟล์ log ไม่พบ ก็ไม่ต้องแสดงข้อความ error แต่ข้ามไปอ่านไฟล์อื่น
			continue
		}
		defer file.Close()

		// ใช้ bufio.Scanner เพื่ออ่านไฟล์เป็นบรรทัดๆ
		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			logData = append(logData, scanner.Text())
			logCount++

			// ถ้าจำนวนบรรทัดที่ดึงมาเกิน limit ให้หยุดการดึง log (ถ้า limit != -1)
			if limit != -1 && logCount >= limit {
				break
			}
		}

		// ถ้าจำนวนบรรทัดเกิน limit แล้ว ให้หยุดการอ่านไฟล์
		if limit != -1 && logCount >= limit {
			break
		}
	}

	// ถ้าไม่พบ log ใด ๆ ในช่วงวันที่ที่ระบุ
	if len(logData) == 0 {
		http.Error(w, "No logs found for the given date range", http.StatusNotFound)
		return
	}

	// ถ้าจำเป็นต้องจัดเรียง log
	if sortOrder == "asc" {
		sort.Strings(logData) // จัดเรียงจากน้อยไปมาก (ตามบรรทัดในไฟล์)
	} else {
		sort.Sort(sort.Reverse(sort.StringSlice(logData))) // จัดเรียงจากมากไปน้อย
	}

	// ถ้าจำนวนบรรทัดที่ดึงมาเกิน limit (ถ้า limit != -1) ให้ตัดข้อมูลที่เกินออก
	if limit != -1 && len(logData) > limit {
		logData = logData[:limit]
	}

	// กำหนดหัวข้อ HTTP response เป็น JSON และส่งข้อมูล log กลับ
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(strings.Join(logData, "\n")))
}

// GetLastLogHandler สำหรับดึงข้อมูล log ล่าสุดของ instance
func (c *Controller) GetLastLogHandler(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if id == "" {
		http.Error(w, "Missing 'id' parameter", http.StatusBadRequest)
		return
	}

	// กำหนด path ของไฟล์ log ล่าสุด
	logFilePath := filepath.Join("logs", id, fmt.Sprintf("%s_log_%s.log", id, time.Now().Format("2006-01-02")))

	// เปิดไฟล์ log
	file, err := os.Open(logFilePath)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to open log file: %v", err), http.StatusInternalServerError)
		return
	}
	defer file.Close()

	// ใช้ bufio.Scanner เพื่ออ่านไฟล์เป็นบรรทัดๆ
	scanner := bufio.NewScanner(file)
	var lastLog string

	// อ่านบรรทัดสุดท้ายของ log
	for scanner.Scan() {
		lastLog = scanner.Text() // เก็บบรรทัดล่าสุดที่อ่าน
	}

	// ถ้าไม่พบ log ใด ๆ
	if lastLog == "" {
		http.Error(w, "No logs found", http.StatusNotFound)
		return
	}

	// กำหนดหัวข้อ HTTP response เป็น JSON และส่งข้อมูล log ล่าสุดกลับ
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(lastLog))
}

// GetErrorLogHandler สำหรับดึงข้อมูล error log ของ instance ในช่วงวันที่ที่กำหนด พร้อมกับจำกัดจำนวนบรรทัดที่ดึงมา
func (c *Controller) GetErrorLogHandler(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if id == "" {
		http.Error(w, "Missing 'id' parameter", http.StatusBadRequest)
		return
	}

	// รับ query parameters สำหรับช่วงวันที่ (start_date, end_date)
	startDate := r.URL.Query().Get("start_date")
	endDate := r.URL.Query().Get("end_date")
	limitParam := r.URL.Query().Get("limit")

	// ถ้าไม่ได้ระบุวันที่เริ่มต้นและวันที่สิ้นสุด ให้ใช้วันที่ปัจจุบัน
	if startDate == "" {
		startDate = time.Now().Format("2006-01-02")
	}
	if endDate == "" {
		endDate = time.Now().Format("2006-01-02")
	}

	// แปลงวันที่ที่ได้รับเป็น time.Time
	start, err := time.Parse("2006-01-02", startDate)
	if err != nil {
		http.Error(w, fmt.Sprintf("Invalid start_date format: %v", err), http.StatusBadRequest)
		return
	}

	end, err := time.Parse("2006-01-02", endDate)
	if err != nil {
		http.Error(w, fmt.Sprintf("Invalid end_date format: %v", err), http.StatusBadRequest)
		return
	}

	// ตั้งค่า default limit ถ้าไม่มีการระบุ
	limit := -1 // ถ้า limit เป็น -1 หมายถึงไม่จำกัดจำนวนบรรทัด
	if limitParam != "" {
		limit, err = strconv.Atoi(limitParam)
		if err != nil || limit <= 0 {
			http.Error(w, "Invalid 'limit' value", http.StatusBadRequest)
			return
		}
	}

	// กำหนด path ของโฟลเดอร์ logs/[id]_error
	var logData []string
	logCount := 0
	for current := start; !current.After(end); current = current.Add(24 * time.Hour) {
		// คำนวณ path ของไฟล์ log สำหรับแต่ละวันในโฟลเดอร์ logs/[id]_error
		logFilePath := filepath.Join("logs", fmt.Sprintf("%s_error", id), fmt.Sprintf("%s_error_log_%s.log", id, current.Format("2006-01-02")))

		// เปิดไฟล์ log
		file, err := os.Open(logFilePath)
		if err != nil {
			// ถ้าไฟล์ log ไม่พบ ก็ไม่ต้องแสดงข้อความ error แต่ข้ามไปอ่านไฟล์อื่น
			continue
		}
		defer file.Close()

		// ใช้ bufio.Scanner เพื่ออ่านไฟล์เป็นบรรทัดๆ
		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			logData = append(logData, scanner.Text())
			logCount++

			// ถ้าจำนวนบรรทัดที่ดึงมาเกิน limit ให้หยุดการดึง log (ถ้า limit != -1)
			if limit != -1 && logCount >= limit {
				break
			}
		}

		// ถ้าจำนวนบรรทัดเกิน limit แล้ว ให้หยุดการอ่านไฟล์
		if limit != -1 && logCount >= limit {
			break
		}
	}

	// ถ้าไม่พบ log ใด ๆ ในช่วงวันที่ที่ระบุ
	if len(logData) == 0 {
		http.Error(w, "No logs found for the given date range", http.StatusNotFound)
		return
	}

	// กำหนดหัวข้อ HTTP response เป็น JSON และส่งข้อมูล log กลับ
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(strings.Join(logData, "\n")))
}

// DeleteOldLogsHandler สำหรับลบ log files ที่มีอายุมากกว่าวันที่กำหนด
func (c *Controller) DeleteOldLogsHandler(w http.ResponseWriter, r *http.Request) {
	// รับ query parameters
	id := r.URL.Query().Get("id")
	retentionParam := r.URL.Query().Get("retention_days")

	if id == "" {
		http.Error(w, "Missing 'id' parameter", http.StatusBadRequest)
		return
	}

	if retentionParam == "" {
		http.Error(w, "Missing 'retention_days' parameter", http.StatusBadRequest)
		return
	}

	// แปลง retention_days เป็นจำนวนเต็ม
	retentionDays, err := strconv.Atoi(retentionParam)
	if err != nil || retentionDays < 0 {
		http.Error(w, "Invalid 'retention_days' value", http.StatusBadRequest)
		return
	}

	// คำนวณวันหมดอายุ
	cutoffDate := time.Now().AddDate(0, 0, -retentionDays)

	// Path ของโฟลเดอร์ log ของ instance
	logDir := filepath.Join("logs", id)

	// อ่านไฟล์ทั้งหมดในโฟลเดอร์ log
	files, err := ioutil.ReadDir(logDir)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to read log directory: %v", err), http.StatusInternalServerError)
		return
	}

	// ลบไฟล์ log ที่เก่ากว่ากำหนด
	var deletedFiles []string
	for _, file := range files {
		if !file.IsDir() {
			// Parse วันที่จากชื่อไฟล์
			fileDateStr := strings.TrimSuffix(strings.TrimPrefix(file.Name(), id+"_log_"), ".log")
			fileDate, err := time.Parse("2006-01-02", fileDateStr)
			if err == nil && fileDate.Before(cutoffDate) {
				// ลบไฟล์
				filePath := filepath.Join(logDir, file.Name())
				if err := os.Remove(filePath); err != nil {
					http.Error(w, fmt.Sprintf("Failed to delete file: %s", filePath), http.StatusInternalServerError)
					return
				}
				deletedFiles = append(deletedFiles, file.Name())
			}
		}
	}

	// ถ้าไม่มีไฟล์ที่ถูกลบ ให้คืนค่าเป็น [] (empty slice)
	if len(deletedFiles) == 0 {
		deletedFiles = []string{}
	}

	// ส่งคำตอบกลับ
	response := map[string]interface{}{
		"deleted_files": deletedFiles,
		"message":       fmt.Sprintf("Deleted %d log files older than %d days", len(deletedFiles), retentionDays),
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// DeleteLogsByDateRangeHandler สำหรับลบ log files ในช่วงวันที่ที่กำหนด
func (c *Controller) DeleteLogsByDateRangeHandler(w http.ResponseWriter, r *http.Request) {
	// รับ query parameters
	id := r.URL.Query().Get("id")
	startDateStr := r.URL.Query().Get("start_date") // วันที่เริ่มต้น
	endDateStr := r.URL.Query().Get("end_date")     // วันที่สิ้นสุด

	if id == "" {
		http.Error(w, "Missing 'id' parameter", http.StatusBadRequest)
		return
	}

	// ถ้าไม่ได้ระบุวันที่เริ่มต้นและวันที่สิ้นสุด ให้ใช้วันนี้
	if startDateStr == "" {
		startDateStr = time.Now().Format("2006-01-02")
	}
	if endDateStr == "" {
		endDateStr = time.Now().Format("2006-01-02")
	}

	// แปลงวันที่เป็น time.Time
	startDate, err := time.Parse("2006-01-02", startDateStr)
	if err != nil {
		http.Error(w, fmt.Sprintf("Invalid start_date format: %v", err), http.StatusBadRequest)
		return
	}

	endDate, err := time.Parse("2006-01-02", endDateStr)
	if err != nil {
		http.Error(w, fmt.Sprintf("Invalid end_date format: %v", err), http.StatusBadRequest)
		return
	}

	// ตรวจสอบว่า start_date ไม่สามารถมากกว่า end_date ได้
	if startDate.After(endDate) {
		http.Error(w, "'start_date' cannot be greater than 'end_date'", http.StatusBadRequest)
		return
	}

	// Path ของโฟลเดอร์ log ของ instance
	logDir := filepath.Join("logs", id)

	// อ่านไฟล์ทั้งหมดในโฟลเดอร์ log
	files, err := ioutil.ReadDir(logDir)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to read log directory: %v", err), http.StatusInternalServerError)
		return
	}

	// ลบไฟล์ log ที่อยู่ในช่วงวันที่ที่กำหนด
	var deletedFiles []string
	for _, file := range files {
		if !file.IsDir() {
			// Parse วันที่จากชื่อไฟล์
			fileDateStr := strings.TrimSuffix(strings.TrimPrefix(file.Name(), id+"_log_"), ".log")
			fileDate, err := time.Parse("2006-01-02", fileDateStr)
			if err == nil && (fileDate.Equal(startDate) || fileDate.Equal(endDate) || (fileDate.After(startDate) && fileDate.Before(endDate))) {
				// ลบไฟล์
				filePath := filepath.Join(logDir, file.Name())
				if err := os.Remove(filePath); err != nil {
					http.Error(w, fmt.Sprintf("Failed to delete file: %s", filePath), http.StatusInternalServerError)
					return
				}
				deletedFiles = append(deletedFiles, file.Name())
			}
		}
	}

	// ลบไฟล์ log ล่าสุดด้วย (ถ้าต้องการ)
	// หาวันล่าสุดในไฟล์ทั้งหมดที่มี
	var latestFile string
	var latestDate time.Time
	for _, file := range files {
		if !file.IsDir() {
			fileDateStr := strings.TrimSuffix(strings.TrimPrefix(file.Name(), id+"_log_"), ".log")
			fileDate, err := time.Parse("2006-01-02", fileDateStr)
			if err == nil && fileDate.After(latestDate) {
				latestDate = fileDate
				latestFile = file.Name()
			}
		}
	}

	if latestFile != "" {
		// ลบไฟล์ล่าสุด
		latestFilePath := filepath.Join(logDir, latestFile)
		if err := os.Remove(latestFilePath); err != nil {
			http.Error(w, fmt.Sprintf("Failed to delete latest file: %s", latestFilePath), http.StatusInternalServerError)
			return
		}
		deletedFiles = append(deletedFiles, latestFile)
	}

	// ถ้าไม่มีไฟล์ที่ถูกลบ ให้คืนค่าเป็น [] (empty slice)
	if len(deletedFiles) == 0 {
		deletedFiles = []string{}
	}

	// ส่งคำตอบกลับ
	response := map[string]interface{}{
		"deleted_files": deletedFiles,
		"message":       fmt.Sprintf("Deleted %d log files in date range %s to %s", len(deletedFiles), startDateStr, endDateStr),
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}
