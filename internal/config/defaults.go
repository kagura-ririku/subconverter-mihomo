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

func defaultCityKeywords() []CityKeyword {
	return []CityKeyword{
		{Country: "TWN", City: "Taipei", Keywords: []string{"台北", "臺北", "Taipei", "TPE"}},
		{Country: "TWN", City: "Taichung", Keywords: []string{"台中", "臺中", "Taichung"}},
		{Country: "TWN", City: "Kaohsiung", Keywords: []string{"高雄", "Kaohsiung"}},
		{Country: "TWN", City: "Tainan", Keywords: []string{"台南", "臺南", "Tainan"}},
		{Country: "TWN", City: "Hsinchu", Keywords: []string{"新竹", "Hsinchu"}},
		{Country: "JPN", City: "Tokyo", Keywords: []string{"东京", "Tokyo", "TYO", "HND", "NRT"}},
		{Country: "JPN", City: "Osaka", Keywords: []string{"大阪", "Osaka", "KIX"}},
		{Country: "JPN", City: "Yokohama", Keywords: []string{"横滨", "横浜", "Yokohama"}},
		{Country: "JPN", City: "Nagoya", Keywords: []string{"名古屋", "Nagoya"}},
		{Country: "JPN", City: "Fukuoka", Keywords: []string{"福冈", "福岡", "Fukuoka"}},
		{Country: "JPN", City: "Saitama", Keywords: []string{"埼玉", "Saitama"}},
		{Country: "KOR", City: "Seoul", Keywords: []string{"首尔", "首爾", "Seoul", "SEL", "ICN"}},
		{Country: "KOR", City: "Busan", Keywords: []string{"釜山", "Busan", "Pusan"}},
		{Country: "KOR", City: "Incheon", Keywords: []string{"仁川", "Incheon"}},
		{Country: "KOR", City: "Daejeon", Keywords: []string{"大田", "Daejeon"}},
		{Country: "KOR", City: "Chuncheon", Keywords: []string{"春川", "Chuncheon"}},
		{Country: "USA", City: "Los Angeles", Keywords: []string{"洛杉矶", "Los Angeles", "LosAngeles", "LAX"}},
		{Country: "USA", City: "San Jose", Keywords: []string{"圣何塞", "圣荷西", "San Jose", "SanJose", "Silicon Valley", "SiliconValley", "硅谷", "SJC"}},
		{Country: "USA", City: "Seattle", Keywords: []string{"西雅图", "Seattle"}},
		{Country: "USA", City: "New York", Keywords: []string{"纽约", "New York", "NewYork", "NYC"}},
		{Country: "USA", City: "Chicago", Keywords: []string{"芝加哥", "Chicago"}},
		{Country: "USA", City: "Dallas", Keywords: []string{"达拉斯", "Dallas", "DFW"}},
		{Country: "USA", City: "Miami", Keywords: []string{"迈阿密", "Miami", "MIA"}},
		{Country: "USA", City: "Phoenix", Keywords: []string{"凤凰城", "Phoenix", "PHX"}},
		{Country: "USA", City: "Fremont", Keywords: []string{"弗里蒙特", "Fremont"}},
		{Country: "USA", City: "Las Vegas", Keywords: []string{"拉斯维加斯", "Las Vegas", "LasVegas", "LAS"}},
		{Country: "USA", City: "Ashburn", Keywords: []string{"阿什本", "Ashburn"}},
		{Country: "USA", City: "Atlanta", Keywords: []string{"亚特兰大", "Atlanta", "ATL"}},
		{Country: "USA", City: "Denver", Keywords: []string{"丹佛", "Denver"}},
	}
}
