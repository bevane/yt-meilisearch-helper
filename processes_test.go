package main

import (
	"reflect"
	"testing"
)

func TestAddNewVideosToProcessing(t *testing.T) {
	var tests = []struct {
		inputVideoList string
		inputStatus    VideoProcessingStatus
		want           VideoProcessingStatus
	}{
		{
			`4rIWeC3YlFA
QNInjEovZEA
w9KtuTH3etE
514v9D1ASSo
`,
			VideoProcessingStatus{},
			VideoProcessingStatus{
				"4rIWeC3YlFA": "pending",
				"QNInjEovZEA": "pending",
				"w9KtuTH3etE": "pending",
				"514v9D1ASSo": "pending",
			},
		},
		{
			"",
			VideoProcessingStatus{},
			VideoProcessingStatus{},
		},
	}
	for _, tt := range tests {
		testname := tt.inputVideoList
		t.Run(testname, func(t *testing.T) {
			addNewVideosToProcessing(tt.inputVideoList, tt.inputStatus)
			if !reflect.DeepEqual(tt.inputStatus, tt.want) {
				t.Errorf(
					"got %v\nwant %v",
					tt.inputStatus, tt.want,
				)
			}

		})
	}

}
