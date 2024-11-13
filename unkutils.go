package main

import (
	"encoding/json"
	"errors"
	"log"
	"os"
	"strconv"
	"strings"
)

func writeProto(protoName string, fields map[int32]string, additional string, prefix string) {
	str := "syntax = \"proto3\";\n\n" + prefix + "message " + protoName + " {\n"
	for tag, typename := range fields {
		str += "  " + typename + " = " + strconv.Itoa(int(tag)) + ";\n"
	}
	if additional != "" {
		str += additional + "\n"
	}
	str += "}\n"

	logHeader := "------------ " + protoName + ".proto" + " ------------"
	log.Println(logHeader)
	log.Println(str)
	log.Println(strings.Repeat("-", len(logHeader)))

	err := os.MkdirAll("./out/", os.ModePerm)
	if err != nil {
		log.Println("Error creating directory in writeProto:", err)
	}
	file, err := os.Create("./out/" + protoName + ".proto")
	if err != nil {
		log.Println("Error creating file in writeProto:", err)
	}
	_, err = file.WriteString(str)
	if err != nil {
		log.Println("Error writing file in writeProto:", err)
	}
	err = file.Close()
	if err != nil {
		log.Println("Error closing file in writeProto:", err)
	}
}

func writePacketIds(packetIds map[uint16]string) {
	err := os.MkdirAll("./out/", os.ModePerm)
	if err != nil {
		log.Println("Error creating directory in writePacketIds:", err)
	}
	file, err := os.Create("./out/packetIds.json")
	if err != nil {
		log.Println("Error creating file in writePacketIds:", err)
	}
	jsonData, err := json.Marshal(packetIds)
	if err != nil {
		log.Println("Error marshaling to JSON in writePacketIds:", err)
		return
	}
	_, err = file.Write(jsonData)
	if err != nil {
		log.Println("Error writing to file in writePacketIds:", err)
		return
	}
	err = file.Close()
	if err != nil {
		log.Println("Error closing file in writePacketIds:", err)
	}
}

func unkPlayerGetTokenScRsp(data []byte) (uint64, error) {
	dMsg, err := parseUnkProto(data)
	if err != nil {
		log.Println("ParseUnkProto", err)
		return 0, err
	}
	var possibleServerSeeds = make(map[int32]uint64)
	// Get the unknown fields from the dynamic message
	unknownFieldTags := dMsg.GetUnknownFields()
	// Iterate over unknown fields
	for i := 0; i < len(unknownFieldTags); i++ {
		tag := unknownFieldTags[i]
		fields := dMsg.GetUnknownField(tag)
		if len(fields) > 1 {
			continue
		}
		field := fields[0]
		// Check if the field is of type bytes and has a length of 64
		seed := field.Value
		if field.Encoding == 0 && seed > 1<<56 { // len(field.Contents) == 344 {
			possibleServerSeeds[tag] = seed
			log.Println("Possible server seed", seed)
		}
	}
	if len(possibleServerSeeds) == 0 || len(possibleServerSeeds) > 1 {
		return 0, errors.New("no possible server seed found")
	}
	for k, v := range possibleServerSeeds {
		writeProto("PlayerGetTokenScRsp", map[int32]string{k: "uint64 secret_key_seed"}, "", "")
		LoadProto("PlayerGetTokenScRsp")
		log.Println("possibleServerSeeds:", possibleServerSeeds)
		return v, nil
	}
	return 0, nil // never happens, for the compiler
}

func unkGetQuestDataScRsp(data []byte) error {
	errs := errors.New("not unkAchievementAllDataNotify")
	dMsg, err := parseUnkProto(data)
	if err != nil {
		log.Println("ParseUnkProto", err)
		return err
	}

	foundId := false
	foundTimestamp := false
	foundStatus := false
	var questListTag int32 = -1
	questFields := make(map[int32]string)

	unknownFieldTags := dMsg.GetUnknownFields()
	for i := 0; i < len(unknownFieldTags); i++ {
		tag := unknownFieldTags[i]
		fields := dMsg.GetUnknownField(tag)
		if len(fields) <= 1 || fields[0].Encoding != 2 {
			continue
		} else if len(questFields) > 0 {
			return errs
		}
		questFieldList := make(map[int32][]uint64)
		for _, field := range fields { // elements of achievement_list
			dMsg2, err := parseUnkProto(field.Contents)
			if err != nil {
				log.Println("ParseUnkProto", err)
				return err
			}
			unknownFieldTags2 := dMsg2.GetUnknownFields()
			if len(unknownFieldTags2) > 5 {
				return errs
			}
			for j := 0; j < len(unknownFieldTags2); j++ { // Fields of Achievement
				tag2 := unknownFieldTags2[j]
				fields2 := dMsg2.GetUnknownField(tag2)
				if len(fields2) > 1 {
					return errs
				}
				field2 := fields2[0]
				questFieldList[tag2] = append(questFieldList[tag2], field2.Value)
			}
		}

		// find fields
		for questTag, lst := range questFieldList {
			isTimestamp := lst[0] > 1420066800 // Wed Dec 31 2014 23:00:00 GMT+0000
			if isTimestamp {
				if foundTimestamp {
					return errs
				}
				questFields[questTag] = "int64 finish_time"
				foundTimestamp = true
				continue
			}
			if len(lst) < 1000 {
				continue
			}
			canBeStatus := true
			isId := false
			for _, val := range lst {
				if val < 0 || val > 4 {
					canBeStatus = false
				}
				if val == 4040201 { // Until the Light Takes Us: Activate 5 Space Anchors in the Herta Space Station
					isId = true
					break
				}
			}
			if isId {
				if foundId {
					return errs
				}
				questFields[questTag] = "uint32 id"
				foundId = true
			} else if canBeStatus {
				if foundStatus {
					return errs
				}
				questFields[questTag] = "QuestStatus status"
				foundStatus = true
			}
		}
		if !foundStatus || !foundId || !foundTimestamp {
			continue
		}
		questListTag = tag
	}
	if !foundStatus || !foundId || !foundTimestamp {
		return errs
	}
	writeProto("Quest", questFields, "  enum QuestStatus {\n    QUEST_NONE = 0;\n    QUEST_DOING = 1;\n    QUEST_FINISH = 2;\n    QUEST_CLOSE = 3;\n    QUEST_DELETE = 4;\n  }", "")
	writeProto("GetQuestDataScRsp", map[int32]string{questListTag: "repeated Quest quest_list"}, "", "import \"Quest.proto\";\n\n")
	return nil
}
