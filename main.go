package main

import (
	"cmp"
	"fmt"
	"log"
	"slices"
	"time"

	"github.com/karalabe/hid"
)

const CodeTemperatureAmbient = 0x42
const CodeCO2RelativeConcentration = 0x50
const TargetVendorID = 0x04d9
const TargetProductID = 0xa052

type Stat struct {
	co2  int
	temp float32
}

func printStat(stat Stat) {
	log.Printf("@ %.4f°C, %d PPM\n", stat.temp, stat.co2)
}

func main() {
	hids, err := hid.Enumerate(0, 0)

	if err != nil {
		panic(err)
	}

	slices.SortFunc(hids, func(a hid.DeviceInfo, b hid.DeviceInfo) int {
		return cmp.Compare(a.Path, b.Path)
	})

	fmt.Printf("hid.Supported() %v\n", hid.Supported())

	for _, hiDev := range hids {
		if hiDev.VendorID == TargetVendorID &&
			hiDev.ProductID == TargetProductID {
			device, err := hiDev.Open()

			if err != nil {
				log.Fatal(err)
			}

			defer device.Close()

			decodeData := hiDev.Release <= 0x100
			magicTable := [8]byte{}
			_, err = device.SendFeatureReport(magicTable[:])

			if err != nil {
				log.Fatalln(err)
			}

			_, err = device.GetFeatureReport(magicTable[:])

			if err != nil {
				log.Fatalln(err)
			}

			stat := Stat{}
			notify := make(chan Stat)

			go func() {
				var state Stat
				ticker := time.NewTicker(5000 * time.Millisecond)
				printStat(state)
				for {
					select {
					case event := <-notify:
						state = event
					case <-ticker.C:
						printStat(state)
					}
				}
			}()

			for {
				readBytes := make([]byte, 8)
				readBytesCount, err := device.ReadTimeout(readBytes, 5000)

				if err != nil {
					log.Fatalln(err)
				}

				if readBytesCount < 1 {
					log.Fatalln("No bytes read")
				}

				if readBytesCount != 8 {
					log.Fatalln("Invalid bytes read count:", readBytesCount)
				}

				// fmt.Println("bytes read:", readBytes)
				result := make([]byte, 8)

				if decodeData {
					swap := func(i, j int) {
						readBytes[i], readBytes[j] = readBytes[j], readBytes[i]
					}
					swap(0, 2)
					swap(1, 4)
					swap(3, 7)
					swap(5, 6)

					for i := range 8 {
						readBytes[i] ^= magicTable[i]
					}

					var tmp byte = (readBytes[7] << 5)
					result[7] = (readBytes[6] << 5) | (readBytes[7] >> 3)
					result[6] = (readBytes[5] << 5) | (readBytes[6] >> 3)
					result[5] = (readBytes[4] << 5) | (readBytes[5] >> 3)
					result[4] = (readBytes[3] << 5) | (readBytes[4] >> 3)
					result[3] = (readBytes[2] << 5) | (readBytes[3] >> 3)
					result[2] = (readBytes[1] << 5) | (readBytes[2] >> 3)
					result[1] = (readBytes[0] << 5) | (readBytes[1] >> 3)
					result[0] = tmp | (readBytes[0] >> 3)

					magic_word := []byte("Htemp99e")
					for i := range 8 {
						result[i] -= (magic_word[i] << 4) | (magic_word[i] >> 4)
					}
				} else {
					copy(result, readBytes)
				}

				if result[4] != 0x0d {
					log.Fatalln("Unexpected data from result[4]")
				}

				r0, r1, r2, r3 := result[0], result[1], result[2], result[3]
				checksum := r0 + r1 + r2

				if checksum != r3 {
					log.Fatalln("Invalid checksum")
				}

				w := uint16(r1)<<8 + uint16(r2)

				switch r0 {
				case CodeTemperatureAmbient:
					stat.temp = float32(w)*0.0625 - 273.15
					notify <- stat
				case CodeCO2RelativeConcentration:
					if w <= 3000 {
						stat.co2 = int(w)
						notify <- stat
					}
				}
			}

			break
		}
	}

}
