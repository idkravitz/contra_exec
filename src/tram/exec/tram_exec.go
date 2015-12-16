package main

import (
	"io"
	"os"
	"os/exec"
	"fmt"
	"log"
	"path"
	"time"
	"path/filepath"
	"tram-commons/lib/model"
	"tram-commons/lib/db"
	"tram-commons/lib/util"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"github.com/streadway/amqp"
	)

const EXEC_PATH = "/home/tram/exec_dir"
var SRC_DIR string = path.Join(EXEC_PATH, "src")
var RUN_DIR string = path.Join(EXEC_PATH, "run")

type TramExecApp struct {
	client_id string
	console_launch bool
	s *mgo.Session
	q *amqp.Connection
}

type DirSpec map[string] time.Time

func prepare_exec_dir() {
	os.RemoveAll(EXEC_PATH)
	os.Mkdir(EXEC_PATH, 0700)
	os.Mkdir(SRC_DIR, 0700)
	os.Mkdir(RUN_DIR, 0700)
}

func (app *TramExecApp) retrieve_file(id string, collection string, dir string, executable bool) *model.FileDescription{
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

func guess_unpack_command(workdir, full_filename string) (* exec.Cmd) {
	ext := filepath.Ext(full_filename)
	var cmd *exec.Cmd;
	switch ext {
	case ".tar":
		cmd = exec.Command("tar", "-xf", full_filename)
	case ".gz", ".gzip":
		cmd = exec.Command("tar", "-xzf", full_filename)
	case ".7z", ".7zip":
		cmd = exec.Command("7za", "x", full_filename)
	}
	cmd.Dir = workdir
	return cmd
}

func unpack_data(workdir, srcdir, filename string) {
	full_filename :=  path.Join(srcdir, filename)
	cmd := guess_unpack_command(workdir, full_filename)

	out, err := cmd.CombinedOutput()
	log.Println(string(out))
	if err != nil {
		log.Fatal(err)
	}
}

func dive_into_data(workdir string) string {
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
		finalPath = dive_into_data(path.Join(finalPath, fis[0].Name()))
	}
	return finalPath
	
}

func Copy(oldpath, newpath string) error {
	fd1, err1 := os.Open(oldpath)
	if err1 != nil {
		return err1
	}
	defer fd1.Close()
	fd2, err2 := os.Create(newpath)
	if err2 != nil {
		return err2
	}
	fd1_stat, err3 := fd1.Stat()
	if err3 != nil {
		return err3
	}
	defer fd2.Close()
	io.Copy(fd2, fd1)

	fd2.Chmod(fd1_stat.Mode())
	return nil
}

func runControlScript(workdir, filename string) ([]byte, error) {
	cmd := exec.Command("/bin/bash", path.Join(workdir, filename))
	cmd.Dir = workdir

	out, err := cmd.CombinedOutput()
	
	return out, err
}

func convert_to_unix(full_filename string) {
	cmd := exec.Command("dos2unix", full_filename)
	err := cmd.Run()
	if err != nil {
		log.Fatal(err)
	}
}

// TODO: share mogno and amqp init section to get rid of init order importance
func createApp() (* TramExecApp) {
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
	app := &TramExecApp{
		s: s,
		q: q,
		client_id: os.Getenv("CLIENT_ID"),
		console_launch: false,
	}
	return app
}

func (app *TramExecApp) Stop() {
	app.s.Close();
	app.q.Close();
}

func placeControlScript(workdir, srcdir, filename string) {
	workdir = dive_into_data(workdir)
	err_copy := Copy(path.Join(srcdir, filename), path.Join(workdir, filename))
	if err_copy != nil {
		log.Fatal(err_copy)
	}
}

func makeDirSpec(dir string) DirSpec {

}

func findChanges(dsa, dsb DirSpec) DirSpec {

}

func (app *TramExecApp) execute(data_fid, control_fid string) ([]byte, error) {
	prepare_exec_dir()
	data_fd := app.retrieve_file(data_fid, "data", SRC_DIR, false)
	control_fd := app.retrieve_file(control_fid, "control", SRC_DIR, true)
	convert_to_unix(path.Join(SRC_DIR, control_fd.Filename))
	unpack_data(RUN_DIR, SRC_DIR, data_fd.Filename)
	placeControlScript(RUN_DIR, SRC_DIR, control_fd.Filename)
	dirSpecBefore := makeDirSpec(RUN_DIR)
	s, e := runControlScript(RUN_DIR, SRC_DIR, control_fd.Filename) 
	dirSpecAfter := makeDirSpec(RUN_DIR)
	return s, e
}

func (app *TramExecApp) processDelivery(delivery amqp.Delivery) {
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

func (app *TramExecApp) MainLoop() {
	channel, err := app.q.Channel()
	if err != nil { // Add durability with redial action
		log.Fatal(err)
	}
	delivery_ch, err2 := channel.Consume("execution_queue", app.client_id, false, false, true, false, nil)
	if err2 != nil {
		log.Fatal(err)
	}
	for {
		delivery := <-delivery_ch
		app.processDelivery(delivery)
	}
}

func main() {
	app := createApp()
	defer app.Stop()
	if len(os.Args) > 1 {
		app.console_launch = true
		data_fid := os.Args[1]
		control_fid := os.Args[2]
		fmt.Println(app.execute(data_fid, control_fid))
	} else {
		app.MainLoop()
	}
}