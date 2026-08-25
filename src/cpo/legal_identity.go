package cpo

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/constants"
)

const gstinAlphabet = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ"

var gstinStateByCode = map[string]constants.IndianState{
	"01": constants.JammuAndKashmir, "02": constants.HimachalPradesh, "03": constants.Punjab, "04": constants.Chandigarh,
	"05": constants.Uttarakhand, "06": constants.Haryana, "07": constants.Delhi, "08": constants.Rajasthan,
	"09": constants.UttarPradesh, "10": constants.Bihar, "11": constants.Sikkim, "12": constants.ArunachalPradesh,
	"13": constants.Nagaland, "14": constants.Manipur, "15": constants.Mizoram, "16": constants.Tripura,
	"17": constants.Meghalaya, "18": constants.Assam, "19": constants.WestBengal, "20": constants.Jharkhand,
	"21": constants.Odisha, "22": constants.Chhattisgarh, "23": constants.MadhyaPradesh, "24": constants.Gujarat,
	"25": constants.DadraAndNagarHaveliAndDamanAndDiu, "26": constants.DadraAndNagarHaveliAndDamanAndDiu,
	"27": constants.Maharashtra, "28": constants.AndhraPradesh, "29": constants.Karnataka, "30": constants.Goa,
	"31": constants.Lakshadweep, "32": constants.Kerala, "33": constants.TamilNadu, "34": constants.Puducherry,
	"35": constants.AndamanAndNicobarIslands, "36": constants.Telangana, "37": constants.AndhraPradesh,
	"38": constants.Ladakh,
}

func normalizeCPOText(value string) string {
	return strings.Join(strings.Fields(value), " ")
}

func validRequiredText(value string, maxRunes int, requireLetter bool) bool {
	if value == "" || !utf8.ValidString(value) || utf8.RuneCountInString(value) > maxRunes {
		return false
	}

	hasLetterOrDigit := false
	for _, character := range value {
		if unicode.IsControl(character) {
			return false
		}
		if unicode.IsLetter(character) || (!requireLetter && unicode.IsDigit(character)) {
			hasLetterOrDigit = true
		}
	}
	return hasLetterOrDigit
}

func validBusinessName(value string) bool { return validRequiredText(value, 255, false) }
func validAddress(value string) bool      { return validRequiredText(value, 5000, false) }
func validCity(value string) bool         { return validRequiredText(value, 100, true) }
func validPersonName(value string) bool   { return validRequiredText(value, 255, true) }

func gstinChecksumCharacter(firstFourteen string) (string, bool) {
	if len(firstFourteen) != 14 {
		return "", false
	}

	sum := 0
	for index, character := range firstFourteen {
		value := strings.IndexRune(gstinAlphabet, character)
		if value < 0 {
			return "", false
		}
		if index%2 == 1 {
			value *= 2
		}
		sum += value/36 + value%36
	}
	return string(gstinAlphabet[(36-sum%36)%36]), true
}

func validGSTINChecksum(gstin string) bool {
	if len(gstin) != 15 {
		return false
	}
	checksum, ok := gstinChecksumCharacter(gstin[:14])
	return ok && gstin[14:] == checksum
}

func validateGSTIN(gstin string, state constants.IndianState) error {
	if !gstinPattern.MatchString(gstin) || !validGSTINChecksum(gstin) {
		return invalid("gstin", "GSTIN must be a valid 15-character Indian GSTIN with a valid checksum.")
	}
	expectedState, ok := gstinStateByCode[gstin[:2]]
	if !ok || expectedState != state {
		return invalid("gstin.state_mismatch", "GSTIN state code must match the CPO registration state.")
	}
	return nil
}
