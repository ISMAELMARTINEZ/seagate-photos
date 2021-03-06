package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/boltdb/bolt"
)

const photosPath = "//192.168.5.8/Seagate/Photos/"

//const photosPath = "C:\\Users\\Ismael\\Pictures\\"

func main() {
	http.HandleFunc("/upload", uploadHandler)
	http.HandleFunc("/list", listHandler)
	http.HandleFunc("/get", getHandler)
	http.Handle("/", http.FileServer(http.Dir("../client/build")))

	//go walkDirectories()
	//Listen on port 3001
	log.Fatal(http.ListenAndServe(":3001", nil))

}

func openDB(readOnly bool) *bolt.DB {
	db, err := bolt.Open(photosPath+"photos.db", 0600, &bolt.Options{Timeout: 1 * time.Second, ReadOnly: readOnly})
	if err != nil {
		log.Fatal(err)
	}
	return db
}

func walkDirectories() {
	db := openDB(false)
	defer db.Close()
	fileList := []string{}
	currentBucket := ""
	err := filepath.Walk(photosPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			log.Printf("prevent panic by handling failure accessing a path %q: %v\n", photosPath, err)
			return err
		}

		fileList = append(fileList, path)
		if info.IsDir() {
			currentBucket = path[len(photosPath):]
			db.Update(func(tx *bolt.Tx) error {
				_, err := tx.CreateBucketIfNotExists([]byte(currentBucket))
				if err != nil {
					log.Printf("create bucket: %v\n", err)
					return err
				}
				return nil
			})
			log.Printf("Entering bucket %q\n", currentBucket)
		} else {
			//ignore the photos.db file else things pertains to music!
			//ignore other files that aren't interesting (like ddesktop.ini)
			if strings.Contains(strings.ToLower(path), ".jpg") || strings.Contains(strings.ToLower(path), ".mp4") {
				db.Update(func(tx *bolt.Tx) error {
					//read file exif info
					b := tx.Bucket([]byte(currentBucket))
					err := b.Put([]byte(path), []byte(getFileInfoFromPathName(path)))
					return err
				})

			}

		}
		return nil
	})

	if err != nil {
		log.Printf("error walking the path %q: %v\n", photosPath, err)
	}
}

//FileInfo structure
type FileInfo struct {
	Name        string
	Size        int64
	Mode        os.FileMode
	ModTime     time.Time
	IsDir       bool
	ContentType string
}

func getFileInfo(pathName string, name string, size int64, mode os.FileMode, modtime time.Time, isDir bool) FileInfo {

	log.Println("reading file", name)
	contentType := "folder"
	if !isDir {
		openFile, err := os.Open(photosPath + pathName + "/" + name)
		if err != nil {
			log.Fatal(err)
		}
		defer openFile.Close()
		buffer := make([]byte, 512)
		_, err = openFile.Read(buffer)
		if err != nil {
			log.Fatal(err)
		}
		contentType = http.DetectContentType(buffer)
	}

	f := FileInfo{
		Name:        name,
		Size:        size,
		Mode:        mode,
		ModTime:     modtime,
		IsDir:       isDir,
		ContentType: contentType,
	}
	return f
}

func getFileInfoFromPathName(pathName string) []byte {

	log.Println("reading file", pathName)
	contentType := "folder"
	openFile, err := os.Open(pathName)

	if err != nil {
		log.Fatal(err)
	}
	defer openFile.Close()
	buffer := make([]byte, 512)
	_, err = openFile.Read(buffer)
	if err != nil {
		log.Fatal(err)
	}
	contentType = http.DetectContentType(buffer)
	fi, err := os.Stat(pathName)
	if err != nil {
		log.Fatal(err)
	}

	finfo := FileInfo{
		Name:        openFile.Name(),
		Size:        fi.Size(),
		Mode:        fi.Mode(),
		ModTime:     fi.ModTime(),
		IsDir:       fi.IsDir(),
		ContentType: contentType,
	}
	result, _ := json.Marshal(finfo)
	return result
}

func getFilesForPath(pathName string) []FileInfo {
	log.Println("Entering getFilesForPath")
	db := openDB(true)
	defer db.Close()
	list := []FileInfo{}
	db.View(func(tx *bolt.Tx) error {
		log.Printf("geting bucket %s", pathName)
		if len(pathName) > 0 {
			b := tx.Bucket([]byte(pathName))
			c := b.Cursor()

			for k, v := c.First(); k != nil; k, v = c.Next() {
				var file FileInfo
				_ = json.Unmarshal(v, &file)
				log.Printf("key=%s, value=%s\n", k, v)
				list = append(list, file)
			}
		} else {
			return tx.ForEach(func(name []byte, _ *bolt.Bucket) error {
				f := getFileInfo(pathName, string(name), 0, 0777, time.Now(), true)

				list = append(list, f)
				log.Println(string(name))
				return nil
			})
		}
		return nil
	})
	return list
}

func getHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		queryValues := r.URL.Query()
		pathName := queryValues.Get("pathname")
		if strings.Compare(pathName, "undefined") == 0 {
			http.Error(w, "no path provided", http.StatusInternalServerError)
			return
		}
		log.Println("Get Method called for getHandler with path", pathName)
		file, err := os.Open(pathName)
		if err != nil {
			log.Fatal(err)
		}
		log.Println(mime.TypeByExtension("." + strings.Split(file.Name(), ".")[1]))
		defer file.Close()
		w.Header().Set("Content-Type", mime.TypeByExtension("."+strings.Split(file.Name(), ".")[1]))
		io.Copy(w, file)
	}
}

func listHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		w.Header().Set("Content-Type", "application/json")
		log.Printf("Get Method called for listHandler")
		queryValues := r.URL.Query()
		pathName := queryValues.Get("pathname")
		log.Println(pathName)
		if strings.Compare(pathName, "undefined") == 0 {
			http.Error(w, "no path provided", http.StatusInternalServerError)
			return
		}
		files := getFilesForPath(pathName)
		output, err := json.Marshal(files)
		if err != nil {
			log.Fatal(err)
		}
		//log.Println(string(output))
		w.Write(output)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func uploadHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "POST":
		reader, err := r.MultipartReader()
		log.Printf("POST upload Method called")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		queryValues := r.URL.Query()
		pathName := queryValues.Get("pathname")

		log.Println(pathName)
		if strings.Compare(pathName, "undefined") == 0 {

			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		for {
			part, err := reader.NextPart()
			if err == io.EOF {
				break
			}

			if part.FileName() == "" {
				continue
			}

			dst, err := os.Create(photosPath + pathName + "/" + part.FileName())
			defer dst.Close()

			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}

			if _, err := io.Copy(dst, part); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		}
		fmt.Fprintf(w, "OK")
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}
