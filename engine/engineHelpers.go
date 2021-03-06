package engine

import (
	"fmt"
	"io"
	"os"
	"time"

	"github.com/anacrolix/torrent"
	"github.com/deranjer/goTorrent/storage"
	"github.com/sirupsen/logrus"
)

func secondsToMinutes(inSeconds int64) string {
	minutes := inSeconds / 60
	seconds := inSeconds % 60
	minutesString := fmt.Sprintf("%d", minutes)
	secondsString := fmt.Sprintf("%d", seconds)
	str := minutesString + " Min/ " + secondsString + " Sec"
	return str
}

//HumanizeBytes returns a nice humanized version of bytes in either GB or MB
func HumanizeBytes(bytes float32) string {
	if bytes < 1000000 { //if we have less than 1MB in bytes convert to KB
		pBytes := fmt.Sprintf("%.2f", bytes/1024)
		pBytes = pBytes + " KB"
		return pBytes
	}
	bytes = bytes / 1024 / 1024 //Converting bytes to a useful measure
	if bytes > 1024 {
		pBytes := fmt.Sprintf("%.2f", bytes/1024)
		pBytes = pBytes + " GB"
		return pBytes
	}
	pBytes := fmt.Sprintf("%.2f", bytes) //If not too big or too small leave it as MB
	pBytes = pBytes + " MB"
	return pBytes
}

//CopyFile takes a source file string and a destination file string and copies the file
func CopyFile(srcFile string, destFile string) {
	fileContents, err := os.Open(srcFile)
	defer fileContents.Close()
	if err != nil {
		Logger.WithFields(logrus.Fields{"File": srcFile, "Error": err}).Error("Cannot open source file")
	}
	outfileContents, err := os.Open(destFile)
	defer outfileContents.Close()
	if err != nil {
		Logger.WithFields(logrus.Fields{"File": destFile, "Error": err}).Error("Cannot open destination file")
	}
	_, err = io.Copy(outfileContents, fileContents)
	if err != nil {
		Logger.WithFields(logrus.Fields{"Source File": srcFile, "Destination File": destFile, "Error": err}).Error("Cannot write contents to destination file")
	}

}

//CalculateTorrentSpeed is used to calculate the torrent upload and download speed over time c is current clientdb, oc is last client db to calculate speed over time
func CalculateTorrentSpeed(t *torrent.Torrent, c *ClientDB, oc ClientDB) {
	now := time.Now()
	bytes := t.BytesCompleted()
	bytesUpload := t.Stats().DataBytesWritten
	dt := float32(now.Sub(oc.UpdatedAt))     // get the delta time length between now and last updated
	db := float32(bytes - oc.BytesCompleted) //getting the delta bytes
	rate := db * (float32(time.Second) / dt) // converting into seconds
	dbU := float32(bytesUpload - oc.DataBytesWritten)
	rateUpload := dbU * (float32(time.Second) / dt)
	if rate >= 0 {
		rate = rate / 1024 / 1024 //creating integer to calculate ETA
		c.DownloadSpeed = fmt.Sprintf("%.2f", rate)
		c.DownloadSpeed = c.DownloadSpeed + " MB/s"
		c.downloadSpeedInt = int64(rate)
	}
	if rateUpload >= 0 {
		rateUpload = rateUpload / 1024 / 1024
		c.UploadSpeed = fmt.Sprintf("%.2f", rateUpload)
		c.UploadSpeed = c.UploadSpeed + " MB/s"

	}
	c.UpdatedAt = now
}

//CalculateTorrentETA is used to estimate the remaining dl time of the torrent based on the speed that the MB are being downloaded
func CalculateTorrentETA(t *torrent.Torrent, c *ClientDB) {
	missingBytes := t.Length() - t.BytesCompleted()
	missingMB := missingBytes / 1024 / 1024
	if missingMB == 0 {
		c.ETA = "Done"
	} else if c.downloadSpeedInt == 0 {
		c.ETA = "N/A"
	} else {
		ETASeconds := missingMB / c.downloadSpeedInt
		str := secondsToMinutes(ETASeconds) //converting seconds to minutes + seconds
		c.ETA = str
	}
}

//CalculateUploadRatio calculates the download to upload ratio so you can see if you are being a good seeder
func CalculateUploadRatio(t *torrent.Torrent, c *ClientDB) string {
	if c.TotalUploadedBytes > 0 && t.BytesCompleted() > 0 { //If we have actually started uploading and downloading stuff start calculating our ratio
		uploadRatio := fmt.Sprintf("%.2f", float64(c.TotalUploadedBytes)/float64(t.BytesCompleted()))
		return uploadRatio
	}
	uploadRatio := "0.00" //we haven't uploaded anything so no upload ratio just pass a string directly
	return uploadRatio
}

//CalculateTorrentStatus is used to determine what the STATUS column of the frontend will display ll2
func CalculateTorrentStatus(t *torrent.Torrent, c *ClientDB, config FullClientSettings, tFromStorage *storage.TorrentLocal) { //TODO redo all of this to allow for stopped torrents
	if (tFromStorage.TorrentStatus == "Stopped") || (float64(c.TotalUploadedBytes)/float64(t.BytesCompleted()) >= config.SeedRatioStop) {
		c.Status = "Stopped"
		c.MaxConnections = 0
		t.SetMaxEstablishedConns(0)
	} else { //Only has 2 states in storage, stopped or running, so we know it should be running, and the websocket request handled updating the database with connections and status
		c.MaxConnections = 80
		t.SetMaxEstablishedConns(80) //TODO this should not be needed but apparently is needed
		t.DownloadAll()              //ensure that we are setting the torrent to download
		if t.Seeding() && t.Stats().ActivePeers > 0 && t.BytesMissing() == 0 {
			c.Status = "Seeding"
		} else if t.Stats().ActivePeers > 0 && t.BytesMissing() > 0 {
			c.Status = "Downloading"
		} else if t.Stats().ActivePeers == 0 && t.BytesMissing() == 0 {
			c.Status = "Completed"
		} else if t.Stats().ActivePeers == 0 && t.BytesMissing() > 0 {
			c.Status = "Awaiting Peers"
		} else {
			c.Status = "Unknown"
		}
	}
}
