/*
Package reporters : report state of kubernetes using MQTT.
	license: Apache license 2.0
	copyright: Nobuyuki Matsui <nobuyuki.matsui@gmail.com>
*/
package reporters

import (
	"fmt"
	"testing"
	"time"

	"go.uber.org/zap"

	apiv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"

	"github.com/tech-sketch/mqtt-kube-operator/mock"
)

func setUpPodStateReporterMocks(t *testing.T, deviceType string, deviceID string, intervalSec int) (*PodStateReporter, func()) {
	ctrl := gomock.NewController(t)

	loggerConfig := zap.NewProductionConfig()
	loggerConfig.Level = zap.NewAtomicLevelAt(zap.FatalLevel)
	logger, _ := loggerConfig.Build()

	podStateReporter := &PodStateReporter{
		baseReporter: &baseReporter{deviceType, deviceID, time.Duration(intervalSec), make(chan bool, 1), make(chan bool, 1)},
		impl:         mock.NewMockReporterImplInf(ctrl),
		logger:       logger.Sugar(),
	}
	return podStateReporter, func() {
		logger.Sync()
		ctrl.Finish()
	}
}

func TestGetTopic(t *testing.T) {
	assert := assert.New(t)

	deviceTypeCases := []struct {
		name string
	}{
		{name: "dType"},
		{name: "/"},
		{name: ""},
	}

	deviceIDCases := []struct {
		name string
	}{
		{name: "dID"},
		{name: "/"},
		{name: ""},
	}

	for _, deviceTypeCase := range deviceTypeCases {
		for _, deviceIDCase := range deviceIDCases {
			t.Run(fmt.Sprintf("deviceType=%v, deviceID=%v", deviceTypeCase.name, deviceIDCase.name), func(t *testing.T) {
				podStateReporter, tearDown := setUpPodStateReporterMocks(t, deviceTypeCase.name, deviceIDCase.name, 1)
				defer tearDown()

				assert.Equal(fmt.Sprintf("/%s/%s/attrs", deviceTypeCase.name, deviceIDCase.name), podStateReporter.GetAttrsTopic())
			})
		}
	}
}

func TestGetChannel(t *testing.T) {
	assert := assert.New(t)
	podStateReporter, tearDown := setUpPodStateReporterMocks(t, "dType", "dID", 1)
	defer tearDown()

	assert.Equal(podStateReporter.stopCh, podStateReporter.GetStopCh())
	assert.Equal(podStateReporter.finishCh, podStateReporter.GetFinishCh())
}

func TestStartReporting(t *testing.T) {
	assert := assert.New(t)
	podStateReporter, tearDown := setUpPodStateReporterMocks(t, "dType", "dID", 10)
	defer tearDown()

	podStateReporter.impl.(*mock.MockReporterImplInf).EXPECT().Report("/dType/dID/attrs").MinTimes(1)
	podStateReporter.StartReporting()

	c := make(chan bool, 1)
	go func() {
		time.Sleep(50 * time.Millisecond)
		podStateReporter.GetStopCh() <- true
		assert.False(<-podStateReporter.GetFinishCh())
		c <- true
	}()
	assert.True(<-c)
}

func setUpPodStateReporterImplMocks(t *testing.T) (*podStateReporterImpl, *mock.MockClient, *mock.MockToken, *mock.MockPodInterface, func()) {
	ctrl := gomock.NewController(t)

	loggerConfig := zap.NewProductionConfig()
	loggerConfig.Level = zap.NewAtomicLevelAt(zap.FatalLevel)
	logger, _ := loggerConfig.Build()

	mqttClient := mock.NewMockClient(ctrl)
	token := mock.NewMockToken(ctrl)

	kubeClient := mock.NewMockInterface(ctrl)
	corev1 := mock.NewMockCoreV1Interface(ctrl)
	podClient := mock.NewMockPodInterface(ctrl)
	kubeClient.EXPECT().CoreV1().Return(corev1).Times(1)
	corev1.EXPECT().Pods(gomock.Any()).Return(podClient).Times(1)

	impl := &podStateReporterImpl{
		logger:         logger.Sugar(),
		mqttClient:     mqttClient,
		kubeClient:     kubeClient,
		targetLabelKey: "test",
		getCurrentTime: func() time.Time {
			return time.Date(2018, 1, 2, 3, 4, 5, 0, time.Local)
		},
	}

	return impl, mqttClient, token, podClient, func() {
		logger.Sync()
		ctrl.Finish()
	}
}

func TestReport(t *testing.T) {
	testCases := []struct {
		podList apiv1.PodList
	}{
		{
			podList: apiv1.PodList{
				Items: []apiv1.Pod{},
			},
		},
		{
			podList: apiv1.PodList{
				Items: []apiv1.Pod{
					{ObjectMeta: metav1.ObjectMeta{Name: "testpod1", Labels: map[string]string{"test": "value1"}}, Status: apiv1.PodStatus{Phase: "Running"}},
				},
			},
		},
		{
			podList: apiv1.PodList{
				Items: []apiv1.Pod{
					{ObjectMeta: metav1.ObjectMeta{Name: "testpod1", Labels: map[string]string{"test": "value1"}}, Status: apiv1.PodStatus{Phase: "Running"}},
					{ObjectMeta: metav1.ObjectMeta{Name: "testpod2", Labels: map[string]string{"test": "value2"}}, Status: apiv1.PodStatus{Phase: "Running"}},
				},
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(fmt.Sprintf("pod num=%d", len(testCase.podList.Items)), func(t *testing.T) {
			impl, mqttClient, token, podClient, tearDown := setUpPodStateReporterImplMocks(t)
			defer tearDown()

			podClient.EXPECT().List(gomock.Any()).Return(&testCase.podList, nil).Times(1)
			if len(testCase.podList.Items) == 0 {
				mqttClient.EXPECT().Publish("/test", byte(0), false, gomock.Any()).Times(0)
			} else if len(testCase.podList.Items) == 1 {
				mqttClient.EXPECT().Publish("/test", byte(0), false, "2018-01-02T03:04:05+09:00|podname|testpod1|podlabel|value1|podphase|Running").Return(token).Times(1)
				token.EXPECT().Wait().Return(true).Times(1)
				token.EXPECT().Error().Return(nil).Times(1)
			} else {
				gomock.InOrder(
					mqttClient.EXPECT().Publish("/test", byte(0), false, "2018-01-02T03:04:05+09:00|podname|testpod1|podlabel|value1|podphase|Running").Return(token),
					mqttClient.EXPECT().Publish("/test", byte(0), false, "2018-01-02T03:04:05+09:00|podname|testpod2|podlabel|value2|podphase|Running").Return(token),
				)
				token.EXPECT().Wait().Return(true).Times(2)
				token.EXPECT().Error().Return(nil).Times(2)
			}
			impl.Report("/test")
		})
	}
}
