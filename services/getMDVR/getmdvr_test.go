package getmdvr

import (
	// "fmt"
	"os"
	"testing"
)

func TestGetAIMDVRLink(t *testing.T) {

	os.Setenv("HOWEN_VIDEO_URL", "https://www.ezview.asia/ezviewnetcore5/MDVR/")
	defer os.Unsetenv("HOWEN_VIDEO_URL")
	
	// Test values
	CscUserId := "10037"
	VehicleId := "94484"
	DateTime := "26/03/2025 10:47:09"
	ViolationId := "4285"
	NotifyType := "ADAS"
	res := ""

	// Call GetAIMDVRLink function
	_, err := GetAIMDVRLink(CscUserId, VehicleId, DateTime, ViolationId, NotifyType, res)
	if err != nil {
		t.Fatalf("Error calling GetAIMDVRLink: %v", err)
	}

}

// package getmdvr

// import (
// 	"fmt"
// 	"net/url"
// 	"testing"
// )

// func TestDecryptURLComponents(t *testing.T) {
// 	fullURL := "https://www.ezview.asia/ezviewnetcore5/MDVR/NotifyMDVR?uid=uxeo01oiZrE=&vid=N15coKlnc7A=&as=0vGC+191DgQ=&dt=N87IFo5Cjh0mF3cFw1md2Q==&at=ftMJjPMGLFs=&lng=th"

// 	parsedURL, err := url.Parse(fullURL)
// 	if err != nil {
// 		t.Fatalf("Failed to parse URL: %v", err)
// 	}

// 	params := parsedURL.Query()
// 	components := []struct {
// 		name      string
// 		encrypted string
// 	}{
// 		{"uid", params.Get("uid")},
// 		{"vid", params.Get("vid")},
// 		{"as", params.Get("as")},
// 		{"dt", params.Get("dt")},
// 		{"at", params.Get("at")},
// 	}

// 	fmt.Println("Decrypted URL Components:")
// 	for _, comp := range components {
// 		decrypted, err := DecryptMD5(comp.encrypted)
// 		if err != nil {
// 			t.Errorf("Decryption failed for %s: %v", comp.name, err)
// 			continue
// 		}
// 		fmt.Printf("%s: %s (encrypted: %s)\n", comp.name, decrypted, comp.encrypted)
// 	}
// }