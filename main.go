package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	controllers "ezview.com/engine/notifications/controllers/v1"
	configServices "ezview.com/engine/notifications/services/configService"
	"ezview.com/engine/notifications/services/logService"
	"github.com/joho/godotenv"

	// "ezview.com/engine/notifications/services/testService"
	notificationServices "ezview.com/engine/notifications/services/notificationService"
	"ezview.com/engine/notifications/services/userService"
)

func withCORS(handler http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}

		handler(w, r)
	}
}

func main() {
	// โหลดค่าจาก .env
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}

	ctrl := &controllers.Controller{
		Mutex: &sync.Mutex{},
		Instances: map[string]*controllers.Instance{
			"entering_no_parking_area":                              {ID: "entering_no_parking_area", EventId: 6002, Task: notificationServices.EnteringNoparkingArea},
			"park_and_stop_outside":                                 {ID: "park_and_stop_outside", EventId: 6004, Task: notificationServices.ParkAndStopOutsideSpot},
			"park_with_engine_running_period_outside_area":          {ID: "park_with_engine_running_period_outside_area", EventId: 6005, Task: notificationServices.ParkedWithEngineRunningExtendedOutsideParking},
			"park_and_turnoff_engine_outside_designated_spot":       {ID: "park_and_turnoff_engine_outside_designated_spot", EventId: 6006, Task: notificationServices.ParkAndTurnOffEngineOutsideDesignatedParkingArea},
			"park_with_engine_running_long_outside_designated_spot": {ID: "park_with_engine_running_long_outside_designated_spot", EventId: 6007, Task: notificationServices.ParkWithEngineRunningLongOutsideDesignatedSpot},
			"pto_off_side":                      {ID: "pto_off_side", EventId: 6008, Task: notificationServices.PTOoffside},
			"over_speed":                        {ID: "over_speed", EventId: 6010, Task: notificationServices.OverSpeedNotification},
			"full_tank_of_oil":                  {ID: "full_tank_of_oil", EventId: 6011, Task: notificationServices.FullTankofOilNotification},
			"tank_is_empty":                     {ID: "tank_is_empty", EventId: 6012, Task: notificationServices.TankIsEmptyNotification},
			"abnomal_oil_levels":                {ID: "abnormal_oil_levels", EventId: 6013, Task: notificationServices.AbnormalOilLevelsNotification},
			"refuel_oil":                        {ID: "refuel_oil", EventId: 6014, Task: notificationServices.RefuelNotifications},
			"over_time_parking":                 {ID: "over_time_parking", EventId: 6015, Task: notificationServices.OverTimeParkingNotification},
			"enter_area":                        {ID: "enter_area", EventId: 6016, Task: notificationServices.EnterAreaNotifications},
			"parked_too_long":                   {ID: "parked_too_long", EventId: 6017, Task: notificationServices.ParkedTooLongNotifications},
			"not_updated_longer":                {ID: "not_updated_longer", EventId: 6018, Task: notificationServices.NotUpdatedLongerThanExpected},
			"exceeding_speed_limit_at_parking":  {ID: "exceeding_speed_limit_at_parking", EventId: 6019, Task: notificationServices.ExceedingSpeedLimitAtParking},
			"emergency_sos":                     {ID: "emergency_sos", EventId: 6009, Task: notificationServices.EmergencyNotifications},
			"entering_and_exiting_the_province": {ID: "entering_and_exiting_the_province", EventId: 6021, Task: notificationServices.EnteringAndExitingTheProvinceNotification},
			"risky_behavior":                    {ID: "risky_behavior", EventId: 6022, Task: notificationServices.RiskyBehaviorNotifications},
			"driving_behavior":                  {ID: "driving_behaviot", EventId: 6023, Task: notificationServices.DrivingBehaviorNotifications},
			"driving_time_exceeded_ad1":         {ID: "driving_time_exceeded_ad1", EventId: 6025, Task: notificationServices.DrivingTimeExceededAD1Notifications},
			"driving_time_exceeded_ad2":         {ID: "driving_time_exceeded_ad2", EventId: 6025, Task: notificationServices.DrivingTimeExceededAD2Notifications},
		},
	}

	// โหลดค่าจากไฟล์ settings
	for id, instance := range ctrl.Instances {
		settings, err := controllers.ReadSettings(id)
		if err != nil {
			fmt.Printf("Settings file for instance %s not found, creating default settings...\n", id)
			if err := configServices.CreateDefaultSettings(id); err != nil {
				fmt.Printf("Failed to create default settings for instance %s: %v\n", id, err)
				continue
			}
			settings, _ = controllers.ReadSettings(id) // โหลด settings ที่สร้างใหม่
		}
		instance.Settings = settings
		autoStart, ok := settings["auto_start"].(bool)
		if ok && autoStart {
			interval, ok := settings["interval"].(float64)
			if !ok || interval <= 0 {
				fmt.Printf("Invalid interval for instance %s\n", id)
				continue
			}
			instance.IsRunning = true
			instance.Done = make(chan bool)
			instance.Ticker = time.NewTicker(time.Duration(interval) * time.Second)

			// ดึงผู้ใช้สำหรับ Instance
			users, err := userService.GetUsersNotifySetting(instance.EventId)
			if err != nil {
				logService.WriteLog(instance.ID+"_error", fmt.Sprintf("Failed to fetch users: %v", err))
				users = []userService.UserNotifySetting{} // ใช้ array ว่างเป็นค่าเริ่มต้นแทน
			}

			instance.Users = users

			// เริ่ม instance โดยส่ง startTime ไปให้ Task
			go func(inst *controllers.Instance) {
				// เก็บเวลาเริ่มต้น
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

			fmt.Printf("Instance %s started automatically with interval %d seconds\n", id, int(interval))
		}
	}

	// Graceful Shutdown
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-signalChan
		fmt.Println("\nShutting down gracefully...")

		// หยุด Instance ทั้งหมด
		for _, instance := range ctrl.Instances {
			close(instance.Done)
		}

		os.Exit(0)
	}()

	// เพิ่ม CORS, Health Check และ API ต่างๆ
	http.HandleFunc("/health", withCORS(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	}))

	// เพิ่ม routes ที่เหลือ
	http.HandleFunc("/start", withCORS(ctrl.StartHandler))
	http.HandleFunc("/stop", withCORS(ctrl.StopHandler))
	http.HandleFunc("/status", withCORS(ctrl.StatusHandler))
	http.HandleFunc("/status/all", withCORS(ctrl.StatusAllHandler))
	http.HandleFunc("/set-settings", withCORS(ctrl.SetSettingsHandler))
	http.HandleFunc("/logs", withCORS(ctrl.GetLogHandler))
	http.HandleFunc("/logs/last", withCORS(ctrl.GetLastLogHandler))
	http.HandleFunc("/logs/error", withCORS(ctrl.GetErrorLogHandler))
	http.HandleFunc("/logs/delete-old", withCORS(ctrl.DeleteOldLogsHandler))
	http.HandleFunc("/logs/delete-by-date-range", withCORS(ctrl.DeleteLogsByDateRangeHandler))

	// อ่านค่า PORT จาก .env หรือค่าปกติ
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	fmt.Println("Server is running on http://localhost:" + port)
	if err := http.ListenAndServe("127.0.0.1:"+port, nil); err != nil {
		log.Fatalf("Server failed: %v", err)
	}

}
