package api_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"

	executorAPI "github.com/cloudfoundry-incubator/executor/api"
	fake_client "github.com/cloudfoundry-incubator/executor/api/fakes"
	"github.com/cloudfoundry-incubator/rep/api"
	"github.com/cloudfoundry-incubator/rep/api/lrprunning"
	"github.com/cloudfoundry-incubator/rep/api/taskcomplete"
	"github.com/cloudfoundry-incubator/rep/routes"
	"github.com/cloudfoundry-incubator/runtime-schema/bbs/fake_bbs"
	"github.com/cloudfoundry-incubator/runtime-schema/models"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/pivotal-golang/lager"
	"github.com/pivotal-golang/lager/lagertest"
	"github.com/tedsuo/rata"
)

var _ = Describe("Callback API", func() {
	var fakeBBS *fake_bbs.FakeRepBBS
	var fakeExecutor *fake_client.FakeClient
	var logger lager.Logger

	var server *httptest.Server
	var httpClient *http.Client

	BeforeSuite(func() {
		logger = lagertest.NewTestLogger("test")

		httpClient = &http.Client{
			Transport: &http.Transport{},
		}
	})

	BeforeEach(func() {
		fakeBBS = &fake_bbs.FakeRepBBS{}
		fakeExecutor = new(fake_client.FakeClient)

		apiServer, err := api.NewServer(
			taskcomplete.NewHandler(fakeBBS, fakeExecutor, logger),
			lrprunning.NewHandler("some-executor-id", fakeBBS, fakeExecutor, "1.2.3.4", logger),
		)
		Ω(err).ShouldNot(HaveOccurred())

		server = httptest.NewServer(apiServer)
	})

	AfterEach(func() {
		server.Close()
	})

	Describe("PUT /task_completed/:guid", func() {
		var task models.Task
		var result executorAPI.ContainerRunResult

		var resp *http.Response

		BeforeEach(func() {
			index := 0

			task = models.Task{
				Guid:       "task-guid-123",
				Stack:      "some-stack",
				MemoryMB:   64,
				DiskMB:     1024,
				CpuPercent: .5,

				ExecutorID:      "some-executor-id",
				ContainerHandle: "some-container-handle",

				Actions: []models.ExecutorAction{
					{
						Action: models.RunAction{
							Path:    "the-script",
							Env:     []models.EnvironmentVariable{{Name: "PATH", Value: "the-path"}},
							Timeout: 500,
						},
					},
				},

				Log: models.LogConfig{
					Guid:       "some-guid",
					SourceName: "XYZ",
					Index:      &index,
				},
			}

			result = executorAPI.ContainerRunResult{
				Guid: "task-guid-123",
			}
		})

		JustBeforeEach(func() {
			body, err := json.Marshal(result)
			Ω(err).ShouldNot(HaveOccurred())

			request, err := http.NewRequest("PUT", server.URL+"/task_completed/"+task.Guid, bytes.NewReader(body))
			Ω(err).ShouldNot(HaveOccurred())

			resp, err = httpClient.Do(request)
			Ω(err).ShouldNot(HaveOccurred())
		})

		Describe("when the task succeeds", func() {
			BeforeEach(func() {
				result.Result = "42"
			})

			It("responds to the onComplete hook", func() {
				Ω(resp.StatusCode).Should(Equal(http.StatusOK))
			})

			It("records the job result", func() {
				Eventually(fakeBBS.CompleteTaskCallCount).Should(Equal(1))
				taskGuid, failed, failureReason, result := fakeBBS.CompleteTaskArgsForCall(0)
				Ω(taskGuid).Should(Equal(task.Guid))
				Ω(failed).Should(BeFalse())
				Ω(failureReason).Should(BeEmpty())
				Ω(result).Should(Equal("42"))
			})
		})

		Describe("when the task fails", func() {
			BeforeEach(func() {
				result.Failed = true
				result.FailureReason = "it didn't work"
			})

			It("responds to the onComplete hook", func() {
				Ω(resp.StatusCode).Should(Equal(http.StatusOK))
			})

			It("records the job failure", func() {
				Eventually(fakeBBS.CompleteTaskCallCount).Should(Equal(1))
				taskGuid, failed, failureReason, result := fakeBBS.CompleteTaskArgsForCall(0)
				Ω(taskGuid).Should(Equal(task.Guid))
				Ω(failed).Should(BeTrue())
				Ω(failureReason).Should(Equal("it didn't work"))
				Ω(result).Should(BeEmpty())
			})
		})
	})

	Describe("PUT /lrp_running/:process_guid/:index/:instance_guid", func() {
		var processGuid string
		var index string
		var instanceGuid string

		var response *http.Response

		BeforeEach(func() {
			processGuid = "some-process-guid"
			index = "2"
			instanceGuid = "some-instance-guid"
		})

		JustBeforeEach(func() {
			generator := rata.NewRequestGenerator(server.URL, routes.Routes)

			request, err := generator.CreateRequest(routes.LRPRunning, rata.Params{
				"process_guid":  processGuid,
				"index":         index,
				"instance_guid": instanceGuid,
			}, nil)
			Ω(err).ShouldNot(HaveOccurred())

			response, err = httpClient.Do(request)
			Ω(err).ShouldNot(HaveOccurred())
		})

		Context("when the guid is found on the executor", func() {
			BeforeEach(func() {
				container := executorAPI.Container{
					Ports: []executorAPI.PortMapping{
						{ContainerPort: 8080, HostPort: 1234},
						{ContainerPort: 8081, HostPort: 1235},
					},
				}
				fakeExecutor.GetContainerReturns(container, nil)
			})

			It("reports the LRP as running", func() {
				actualLRP, executorGUID := fakeBBS.ReportActualLRPAsRunningArgsForCall(0)
				Ω(actualLRP).Should(Equal(models.ActualLRP{
					ProcessGuid:  "some-process-guid",
					Index:        2,
					InstanceGuid: "some-instance-guid",

					Host: "1.2.3.4",

					Ports: []models.PortMapping{
						{ContainerPort: 8080, HostPort: 1234},
						{ContainerPort: 8081, HostPort: 1235},
					},
				}))

				Ω(executorGUID).Should(Equal("some-executor-id"))
			})

			It("returns 200", func() {
				Ω(response.StatusCode).Should(Equal(http.StatusOK))
			})
		})

		Context("when the guid is not found on the executor", func() {
			disaster := errors.New("oh no!")

			BeforeEach(func() {
				fakeExecutor.GetContainerReturns(executorAPI.Container{}, disaster)
			})

			It("returns 400", func() {
				Ω(response.StatusCode).Should(Equal(http.StatusBadRequest))
			})
		})

		Context("when the index is not a number", func() {
			BeforeEach(func() {
				index = "nope"
			})

			It("returns 400", func() {
				Ω(response.StatusCode).Should(Equal(http.StatusBadRequest))
			})
		})

		Context("when reporting it as running fails", func() {
			BeforeEach(func() {
				fakeBBS.ReportActualLRPAsRunningReturns(errors.New("oh no!"))
			})

			It("returns 500", func() {
				Ω(response.StatusCode).Should(Equal(http.StatusInternalServerError))
			})
		})
	})
})
