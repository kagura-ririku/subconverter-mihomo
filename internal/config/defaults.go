package config

func defaultRegionOrder() []string {
	return []string{"HKG", "MAC", "TWN", "JPN", "KOR", "SGP", "USA"}
}

func defaultNodeSortRegionOrder() []string {
	return []string{"HKG", "MAC", "TWN", "JPN", "KOR", "SGP", "USA"}
}

func defaultCitylessCountries() map[string]struct{} {
	return map[string]struct{}{
		"HKG": {},
		"MAC": {},
		"SGP": {},
	}
}

func defaultRegionFlags() map[string]string {
	return map[string]string{
		"HKG": "🇭🇰",
		"MAC": "🇲🇴",
		"TWN": "🇹🇼",
		"JPN": "🇯🇵",
		"KOR": "🇰🇷",
		"SGP": "🇸🇬",
		"USA": "🇺🇸",
		"DEU": "🇩🇪",
		"TUR": "🇹🇷",
		"ARG": "🇦🇷",
		"MYS": "🇲🇾",
		"NGA": "🇳🇬",
		"PAK": "🇵🇰",
	}
}

func defaultRegionKeywords() map[string][]string {
	return map[string][]string{
		"HKG": {"🇭🇰", "香港", "港", "HK", "HKG", "Hong Kong", "Hongkong"},
		"MAC": {"🇲🇴", "澳门", "MO", "MAC", "Macau", "Macao"},
		"TWN": {"🇹🇼", "台湾", "台", "TW", "TWN", "Taiwan"},
		"JPN": {"🇯🇵", "日本", "日", "JP", "JPN", "Japan"},
		"KOR": {"🇰🇷", "韩国", "韩", "KR", "KOR", "Korea", "South Korea"},
		"SGP": {"🇸🇬", "新加坡", "狮城", "SG", "SGP", "Singapore"},
		"USA": {"🇺🇸", "美国", "美", "US", "USA", "America", "United States"},
		"DEU": {"🇩🇪", "德国", "德", "DE", "DEU", "Germany", "Frankfurt", "Berlin"},
		"TUR": {"🇹🇷", "土耳其", "TR", "TUR", "Turkey", "Turkiye", "Türkiye", "Istanbul"},
		"ARG": {"🇦🇷", "阿根廷", "AR", "ARG", "Argentina", "Buenos Aires"},
		"MYS": {"🇲🇾", "马来西亚", "馬來西亞", "MY", "MYS", "Malaysia", "Kuala Lumpur"},
		"NGA": {"🇳🇬", "尼日利亚", "尼日利亞", "NG", "NGA", "Nigeria", "Lagos"},
		"PAK": {"🇵🇰", "巴基斯坦", "PK", "PAK", "Pakistan", "Islamabad", "Karachi"},
	}
}

func defaultCityKeywords() map[string][]string {
	return map[string][]string{
		"Taipei":      {"台北", "臺北", "Taipei", "TPE"},
		"Taichung":    {"台中", "臺中", "Taichung"},
		"Kaohsiung":   {"高雄", "Kaohsiung"},
		"Tainan":      {"台南", "臺南", "Tainan"},
		"Hsinchu":     {"新竹", "Hsinchu"},
		"Tokyo":       {"东京", "Tokyo", "TYO", "HND", "NRT"},
		"Osaka":       {"大阪", "Osaka", "KIX"},
		"Yokohama":    {"横滨", "横浜", "Yokohama"},
		"Nagoya":      {"名古屋", "Nagoya"},
		"Fukuoka":     {"福冈", "福岡", "Fukuoka"},
		"Saitama":     {"埼玉", "Saitama"},
		"Seoul":       {"首尔", "首爾", "Seoul", "SEL", "ICN"},
		"Busan":       {"釜山", "Busan", "Pusan"},
		"Incheon":     {"仁川", "Incheon"},
		"Daejeon":     {"大田", "Daejeon"},
		"Chuncheon":   {"春川", "Chuncheon"},
		"Los Angeles": {"洛杉矶", "Los Angeles", "LosAngeles", "LAX"},
		"San Jose":    {"圣何塞", "圣荷西", "San Jose", "SanJose", "Silicon Valley", "SiliconValley", "硅谷", "SJC"},
		"Seattle":     {"西雅图", "Seattle"},
		"New York":    {"纽约", "New York", "NewYork", "NYC"},
		"Chicago":     {"芝加哥", "Chicago"},
		"Dallas":      {"达拉斯", "Dallas", "DFW"},
		"Miami":       {"迈阿密", "Miami", "MIA"},
		"Phoenix":     {"凤凰城", "Phoenix", "PHX"},
		"Fremont":     {"弗里蒙特", "Fremont"},
		"Las Vegas":   {"拉斯维加斯", "Las Vegas", "LasVegas", "LAS"},
		"Ashburn":     {"阿什本", "Ashburn"},
		"Atlanta":     {"亚特兰大", "Atlanta", "ATL"},
		"Denver":      {"丹佛", "Denver"},
	}
}
