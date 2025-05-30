package utilService

// ฟังก์ชันช่วยสำหรับคืนค่า "-" หากไม่มีค่า
func NonEmptyString(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}
