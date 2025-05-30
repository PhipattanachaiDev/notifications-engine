package testService

import (
	"fmt"
	"math/rand"
	"time"

	"ezview.com/engine/notifications/services/userService"
)

func GetGachaNumber() int {
	// น้ำหนัก (weight) ของแต่ละตัวเลขที่ต้องการ
	// ตัวเลข 0 จะมีน้ำหนักสูงสุด
	// ตัวเลข 10 จะมีน้ำหนักต่ำสุด
	weights := []int{100, 80, 60, 50, 40, 30, 25, 20, 15, 10, 5}

	// คำนวณการสุ่มเลขด้วย weighted random
	totalWeight := 0
	for _, weight := range weights {
		totalWeight += weight
	}

	// สุ่มตัวเลขจาก totalWeight
	randomWeight := rand.Intn(totalWeight)

	// คำนวณว่า randomWeight ตกอยู่ในตัวเลขไหน
	currentWeight := 0
	for i, weight := range weights {
		currentWeight += weight
		if randomWeight < currentWeight {
			return i // ส่งตัวเลขที่ถูกเลือก
		}
	}
	return 0
}

func Task(startTime time.Time, settings map[string]interface{}, users *[]userService.UserNotifySetting) {
	fmt.Println("Task started at:", startTime)

	// ตัวอย่างการพิมพ์ข้อมูลผู้ใช้ทั้งหมด
	fmt.Println("Users:")
	for _, user := range *users {
		fmt.Printf("UserID: %d, LastMessageGPSDateTime: %v\n",
			user.UserID, user.LastMessageGPSDateTime.Format(time.RFC3339))
	}

	// ตัวอย่าง: เพิ่มผู้ใช้ใหม่
	newUser := userService.UserNotifySetting{
		UserID:      999,
		IsSendEmail: true,
		Reserve1:    "New Reserve1",
		Reserve2:    "New Reserve2",
	}
	*users = append(*users, newUser)
	fmt.Println("Added new user:", newUser.UserID)

	// ตัวอย่าง: ลบผู้ใช้ที่ซ้ำ
	uniqueUsers := []userService.UserNotifySetting{}
	seen := make(map[int]bool)
	for _, user := range *users {
		if !seen[user.UserID] {
			seen[user.UserID] = true
			uniqueUsers = append(uniqueUsers, user)
		} else {
			fmt.Printf("Duplicate user found and removed: %d\n", user.UserID)
		}
	}
	*users = uniqueUsers

	// ตัวอย่าง: ตรวจสอบผู้ใช้ที่ไม่มีการตั้งค่า `EmailSetting`
	for i, user := range *users {
		if len(user.EmailSetting) == 0 {
			(*users)[i].EmailSetting = []interface{}{"Default email setting"}
			fmt.Printf("Updated EmailSetting for UserID: %d\n", user.UserID)
		}
	}

	// พิมพ์ผู้ใช้หลังปรับปรุง
	fmt.Println("Updated Users:")
	for _, user := range *users {
		fmt.Printf("UserID: %d, IsSendEmail: %t, EmailSetting: %v\n",
			user.UserID, user.IsSendEmail, user.EmailSetting)
	}
}
