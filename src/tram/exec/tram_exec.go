package main

import (
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"time"
	"tram-commons/lib/db"
	"tram-commons/lib/model"
	"tram-commons/lib/util"

	"github.com/streadway/amqp"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
)

const execPath = "/home/tram/exec_dir"

var srcDir = path.Join(execPath, "src")
var runDir = path.Join(execPath, "run")

type tramExecApp struct {
	clientID      string
	consoleLaunch bool
	s             *mgo.Session
	q             *amqp.Connection
}

type dirSpecUnit struct {
	ModTime time.Time
	IsDir   bool
}

type dirSpec map[string]dirSpecUnit

func prepareExecDir() {
	os.RemoveAll(execPath)
	os.Mkdir(execPath, 0700)
	os.Mkdir(srcDir, 0700)
	os.Mkdir(runDir, 0700)
}

func (app *tramExecApp) retrieveFile(id string, collection string, dir string, executable bool) *model.FileDescription {
	s := app.s.Copy()
	defer s.Close()

	file, err := s.DB("tram").GridFS(collection).OpenId(bson.ObjectIdHex(id))
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()
	fd := &model.FileDescription{}

	file.GetMeta(fd)
	filename := path.Join(dir, fd.Filename)
	out, err := os.Create(filename)
	if err != nil {
		log.Fatal(err)
	}
	defer out.Close()
	io.Copy(out, file)
	if executable {
		out.Chmod(0700)
	}
	return fd
}

func guessUnpackCommand(workdir, fullFilename string) *exec.Cmd {
	ext := filepath.Ext(fullFilename)
	var cmd *exec.Cmd
	switch ext {
	case ".tar":
		cmd = exec.Command("tar", "-xf", fullFilename)
	case ".gz", ".gzip":
		cmd = exec.Command("tar", "-xzf", fullFilename)
	case ".7z", ".7zip":
		cmd = exec.Command("7za", "x", fullFilename)
	}
	cmd.Dir = workdir
	return cmd
}

func unpackData(workdir, srcDir, filename string) {
	fullFilename := path.Join(srcDir, filename)
	cmd := guessUnpackCommand(workdir, fullFilename)

	out, err := cmd.CombinedOutput()
	log.Println(string(out))
	if err != nil {
		log.Fatal(err)
	}
}

func diveIntoData(workdir string) string {
	wd, err1 := os.Open(workdir)
	if err1 != nil {
		log.Fatal(err1)
	}
	fis, err2 := wd.Readdir(0)
	if err2 != nil {
		log.Fatal(err2)
	}
	finalPath := workdir
	if len(fis) == 1 && fis[0].IsDir() {
		finalPath = diveIntoData(path.Join(finalPath, fis[0].Name()))
	}
	return finalPath

}

func simpleCopy(oldpath, newpath string) error {
	fd1, err1 := os.Open(oldpath)
	if err1 != nil {
		return err1
	}
	defer fd1.Close()
	fd2, err2 := os.Create(newpath)
	if err2 != nil {
		return err2
	}
	fd1Stat, err3 := fd1.Stat()
	if err3 != nil {
		return err3
	}
	defer fd2.Close()
	io.Copy(fd2, fd1)

	fd2.Chmod(fd1Stat.Mode())
	return nil
}

func runControlScript(workdir, filename string) ([]byte, error) {
	cmd := exec.Command("/bin/bash", path.Join(workdir, filename))
	cmd.Dir = workdir

	out, err := cmd.CombinedOutput()

	return out, err
}

func convertToUnixLE(fullFilename string) {
	cmd := exec.Command("dos2unix", fullFilename)
	err := cmd.Run()
	if err != nil {
		log.Fatal(err)
	}
}

// TODO: share mogno and amqp init section to get rid of init order importance
func createApp() *tramExecApp {
	mongoSocket := "tram-mongo:27017"
	log.Println("Connect to mongo at:", mongoSocket)
	s, err := db.MongoInitConnect(mongoSocket)
	if err != nil {
		panic(err)
	}

	rabbitUser := util.GetenvDefault("RABBIT_USER", "guest")
	rabbitPassword := util.GetenvDefault("RABBIT_PASSWORD", "guest")
	amqpSocket := fmt.Sprintf("amqp://%v:%v@tram-rabbit:5672", rabbitUser, rabbitPassword)
	log.Println("Connect to rabbit at:", amqpSocket)
	q, err2 := db.RabbitInitConnect(amqpSocket)
	if err2 != nil {
		log.Fatal(err2)
	}
	app := &tramExecApp{
		s:             s,
		q:             q,
		clientID:      os.Getenv("clientID"),
		consoleLaunch: false,
	}
	return app
}

func (app *tramExecApp) Stop() {
	app.s.Close()
	app.q.Close()
}

func placeControlScript(workdir, srcDir, filename string) {
	workdir = diveIntoData(workdir)
	err := simpleCopy(path.Join(srcDir, filename), path.Join(workdir, filename))
	if err != nil {
		log.Fatal(err)
	}
}

func fillDirSpec(name string, dirSpec dirSpec) error {

	f, err := os.Open(name)
	if err != nil {
		return err
	}
	fi, err2 := f.Stat()
	if err2 != nil {
		return err2
	}

	dirSpec[name] = dirSpecUnit{
		ModTime: fi.ModTime(),
		IsDir:   fi.IsDir(),
	}
	if fi.IsDir() {
		names, err3 := f.Readdirnames(0)
		if err3 != nil {
			return err3
		}
		for _, name := range names {
			fillDirSpec(name, dirSpec)
		}
	}
	// dirInfo, err = f.Readdir(0)
	// if err != nil {
	// log.Fatal(err)
	// }
	return nil
}

func findChanges(dsa, dsb dirSpec) dirSpec {
	return nil
}

func (app *tramExecApp) execute(dataFid, controlFid string) ([]byte, error) {
	prepareExecDir()
	dataFd := app.retrieveFile(dataFid, "data", srcDir, false)
	controlFd := app.retrieveFile(controlFid, "control", srcDir, true)
	convertToUnixLE(path.Join(srcDir, controlFd.Filename))
	unpackData(runDir, srcDir, dataFd.Filename)
	placeControlScript(runDir, srcDir, controlFd.Filename)

	dirSpecBefore := dirSpec{}
	fillDirSpec(runDir, dirSpecBefore)
	s, e := runControlScript(runDir, controlFd.Filename)
	dirSpecAfter := dirSpec{}
	fillDirSpec(runDir, dirSpecAfter)

	//diff := findChanges(dirSpecBefore, dirSpecAfter)

	return s, e
}

func (app *tramExecApp) processDelivery(delivery amqp.Delivery) {
	msg := model.TaskMsg{}
	if err := bson.Unmarshal(delivery.Body, &msg); err != nil {
		log.Fatal(err)
	}
	output, err := app.execute(msg.DataFid, msg.ControlFid)
	s := app.s.Copy()
	defer s.Close()

	if err := s.DB("tram").C("tasks").UpdateId(msg.TaskId, &bson.M{"$set": &bson.M{"output": string(output), "status": model.TASK_STATUS_DONE}}); err != nil {
		log.Fatal(err)
	}

	// fmt.Println(output)
	if err != nil {
		fmt.Println("!!!Error:", err)
	}
	if err := delivery.Ack(false); err != nil {
		log.Fatal(err)
	}
}

func (app *tramExecApp) MainLoop() {
	channel, err := app.q.Channel()
	if err != nil { // Add durability with redial action
		log.Fatal(err)
	}
	deliveryCh, err2 := channel.Consume("execution_queue", app.clientID, false, false, true, false, nil)
	if err2 != nil {
		log.Fatal(err)
	}
	for {
		delivery := <-deliveryCh
		app.processDelivery(delivery)
	}
}

func main() {
	app := createApp()
	defer app.Stop()
	if len(os.Args) > 1 {
		app.consoleLaunch = true
		dataFid := os.Args[1]
		controlFid := os.Args[2]
		fmt.Println(app.execute(dataFid, controlFid))
	} else {
		app.MainLoop()
	}
}
