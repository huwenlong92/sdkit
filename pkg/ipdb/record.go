package ipdb

type Record struct {
	IP      string `json:"ip,omitempty"`
	Version int    `json:"version,omitempty"`
	Source  string `json:"source,omitempty"`

	Continent     string `json:"continent,omitempty"`
	ContinentCode string `json:"continent_code,omitempty"`

	Country     string `json:"country,omitempty"`
	CountryCode string `json:"country_code,omitempty"`

	Region     string `json:"region,omitempty"`
	RegionCode string `json:"region_code,omitempty"`
	Province   string `json:"province,omitempty"`
	City       string `json:"city,omitempty"`
	District   string `json:"district,omitempty"`
	PostalCode string `json:"postal_code,omitempty"`
	TimeZone   string `json:"time_zone,omitempty"`

	ISP            string `json:"isp,omitempty"`
	ASN            string `json:"asn,omitempty"`
	ASNumber       uint   `json:"as_number,omitempty"`
	ASOrganization string `json:"as_organization,omitempty"`

	Latitude  *float64 `json:"latitude,omitempty"`
	Longitude *float64 `json:"longitude,omitempty"`

	Network               string `json:"network,omitempty"`
	RegisteredCountry     string `json:"registered_country,omitempty"`
	RegisteredCountryCode string `json:"registered_country_code,omitempty"`
}

func (r *Record) HasData() bool {
	if r == nil {
		return false
	}
	return r.Continent != "" ||
		r.Country != "" ||
		r.CountryCode != "" ||
		r.Region != "" ||
		r.Province != "" ||
		r.City != "" ||
		r.District != "" ||
		r.ISP != "" ||
		r.ASN != "" ||
		r.ASNumber != 0 ||
		r.ASOrganization != "" ||
		r.Latitude != nil ||
		r.Longitude != nil ||
		r.Network != "" ||
		r.RegisteredCountry != "" ||
		r.RegisteredCountryCode != ""
}

func mergeRecord(base, next *Record) *Record {
	if base == nil {
		return next
	}
	if next == nil {
		return base
	}

	if base.IP == "" {
		base.IP = next.IP
	}
	if base.Version == 0 {
		base.Version = next.Version
	}
	if base.Source == "" {
		base.Source = next.Source
	} else if next.Source != "" && base.Source != next.Source {
		base.Source += "," + next.Source
	}
	if base.Continent == "" {
		base.Continent = next.Continent
	}
	if base.ContinentCode == "" {
		base.ContinentCode = next.ContinentCode
	}
	if base.Country == "" {
		base.Country = next.Country
	}
	if base.CountryCode == "" {
		base.CountryCode = next.CountryCode
	}
	if base.Region == "" {
		base.Region = next.Region
	}
	if base.RegionCode == "" {
		base.RegionCode = next.RegionCode
	}
	if base.Province == "" {
		base.Province = next.Province
	}
	if base.City == "" {
		base.City = next.City
	}
	if base.District == "" {
		base.District = next.District
	}
	if base.PostalCode == "" {
		base.PostalCode = next.PostalCode
	}
	if base.TimeZone == "" {
		base.TimeZone = next.TimeZone
	}
	if base.ISP == "" {
		base.ISP = next.ISP
	}
	if base.ASN == "" {
		base.ASN = next.ASN
	}
	if base.ASNumber == 0 {
		base.ASNumber = next.ASNumber
	}
	if base.ASOrganization == "" {
		base.ASOrganization = next.ASOrganization
	}
	if base.Latitude == nil {
		base.Latitude = next.Latitude
	}
	if base.Longitude == nil {
		base.Longitude = next.Longitude
	}
	if base.Network == "" {
		base.Network = next.Network
	}
	if base.RegisteredCountry == "" {
		base.RegisteredCountry = next.RegisteredCountry
	}
	if base.RegisteredCountryCode == "" {
		base.RegisteredCountryCode = next.RegisteredCountryCode
	}
	return base
}
