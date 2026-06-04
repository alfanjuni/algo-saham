package helper

import (
	"time"
)

const (
	// Pengaturan Jam Operasional (WIB)
	UseTradingHours = false
	StartHour       = 8
	StartMinute     = 30
	EndHour         = 16
	EndMinute       = 30
)

func IsTradingTime() bool {
	if !UseTradingHours {
		return true
	}

	// Gunakan zona waktu WIB (UTC+7)
	wib := time.FixedZone("WIB", 7*3600)
	now := time.Now().In(wib)

	// Cek hari (Senin-Jumat)
	if now.Weekday() == time.Saturday || now.Weekday() == time.Sunday {
		return false
	}

	currentMinutes := now.Hour()*60 + now.Minute()
	startMinutes := StartHour*60 + StartMinute
	endMinutes := EndHour*60 + EndMinute

	return currentMinutes >= startMinutes && currentMinutes <= endMinutes
}

func GetEffectiveDate() string {
	wib := time.FixedZone("WIB", 7*3600)
	now := time.Now().In(wib)

	// Jika sebelum jam buka (08:30 WIB), gunakan tanggal kemarin
	if now.Hour() < StartHour || (now.Hour() == StartHour && now.Minute() < StartMinute) {
		return now.AddDate(0, 0, -1).Format("2006-01-02")
	}
	return now.Format("2006-01-02")
}
