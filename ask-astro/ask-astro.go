package askastro

import (
	"fmt"
	"path/filepath"

	"github.com/astronomer/astro-cli/pkg/ansi"
	"github.com/astronomer/astro-cli/pkg/fileutil"
	"github.com/astronomer/astro-cli/pkg/input"
	"github.com/pkg/errors"
)

var (
	spinnerMessage string
	response       string
)

type ResponseData struct {
	HealthCheckValues HealthCheckValues `json:"health_check_values"`
	Context           string            `json:"context"`
	CorrectedDAG      string            `json:"corrected_dag"`
}

type HealthCheckValues struct {
	Idempotency               bool `json:"idempotency"`
	AtomicTasks               bool `json:"atomic_tasks"`
	UniqueCode                bool `json:"unique_code"`
	NoTopLevelCode            bool `json:"no_top_level_code"`
	TaskDependencyConsistency bool `json:"task_dependency_consistency"`
	StaticStartDate           bool `json:"static_start_date"`
	RetriesSet                bool `json:"retries_set"`
}

func DAGReview(dagFilePath string, CICD bool) error {

	// fmt.Println("import json\nfrom pendulum import datetime\nfrom airflow.decorators import (\ndag,\ntask,)\n\n@dag(\nschedule='@daily',\nstart_date=datetime(2023, 1, 1),\ncatchup=False,\ndefault_args={'retries': 3, 'retry_delay': timedelta(minutes=5)},\ntags=['example'],)\ndef example_dag_basic():\n\n    @task()\ndef extract():\ndata_string = '{\"1001\": 301.27, \"1002\": 433.21, \"1003\": 502.22}'\norder_data_dict = json.loads(data_string)\nreturn order_data_dict\n\n    @task(multiple_outputs=True)\n\ndef transform(order_data_dict: dict):\ntotal_order_value = 0\nfor value in order_data_dict.values():\ntotal_order_value += value\nreturn {'total_order_value': total_order_value}\n\n    @task()\ndef load(total_order_value: float):\nprint(f'Total order value is: {total_order_value:.2f}')\n\n    order_data = extract()\n    order_summary = transform(order_data)\n    load(order_summary['total_order_value'])\n\nexample_dag_basic()")

	// return nil
	_, err := fileutil.Exists(dagFilePath, nil)
	if err != nil {
		return errors.Wrapf(err, "failed to check existence of '%s'", dagFilePath)
	}

	dagFileContents, err := fileutil.ReadFileToString(dagFilePath)

	requestPrompt := fmt.Sprintf(dagReviewPrompt2, dagFileContents)

	spinnerMessage = fmt.Sprintf("Ask Astro is reviewing %s", filepath.Base(dagFilePath))

	err = ansi.Spinner(spinnerMessage, func() error {
		response, err = askAstroRequest(requestPrompt)
		// time.Sleep(time.Duration(2) * time.Second)
		return err
	})
	if err != nil {
		return err
	}

	// fmt.Printf("%q\n", response)

	// Define a variable to hold the unmarshaled data
	// var data ResponseData
	// // Unmarshal the JSON string into the MyData struct
	// err = json.Unmarshal([]byte(fakeResponse), &data)
	// if err != nil {
	// 	return err
	// }
	// if err != nil {
	// 	fmt.Println("Unable to parse reponse from Ask Astro:")
	// 	fmt.Println("\n" + response)
	// 	return err
	// }

	// fmt.Println("Review Results:")
	// fmt.Println("\tIdempotency: " + passedBool(data.HealthCheckValues.Idempotency))
	// fmt.Println("\tAtomic Tasks: " + passedBool(data.HealthCheckValues.AtomicTasks))
	// fmt.Println("\tUnique Code: " + passedBool(data.HealthCheckValues.UniqueCode))
	// fmt.Println("\tNo Top Level Code: " + passedBool(data.HealthCheckValues.NoTopLevelCode))
	// fmt.Println("\tTask Dependency Consistency: " + passedBool(data.HealthCheckValues.TaskDependencyConsistency))
	// fmt.Println("\tStatic Start Date: " + passedBool(data.HealthCheckValues.StaticStartDate))
	// fmt.Println("\tRetries Set: " + passedBool(data.HealthCheckValues.RetriesSet) + "\n")

	// wrappedString := wordwrap.WrapString(data.Context, 80) // 80 is the desired width

	fmt.Println(response)
	// testFailed, err := checkReview(data, CICD)
	// if err != nil {
	// 	return err
	// }

	// if testFailed {
	// 	err = dagUpdate(data.CorrectedDAG, dagFilePath)
	// 	if err != nil {
	// 		return err
	// 	}
	// }

	return nil
}

func passedBool(testBool bool) string {
	if testBool {
		correct := ansi.Green("✔")
		return correct
	}
	wrong := ansi.Red("x")
	return wrong
}

func checkReview(data ResponseData, CICD bool) (bool, error) {
	values := data.HealthCheckValues
	if !values.Idempotency || !values.AtomicTasks || !values.UniqueCode || !values.NoTopLevelCode || !values.TaskDependencyConsistency || !values.StaticStartDate || !values.RetriesSet {
		if CICD {
			return true, errors.New("\nYour DAG did not pass the review fix the issues outlined above and try again")
		} else {
			return true, nil
		}
	} else {
		fmt.Println("\n" + ansi.Green("✔") + " your DAG passed the review!")
	}
	return false, nil
}

func dagUpdate(correctedDAG, dagFilePath string) error {
	fmt.Printf("\n%s to see a DAG file that contains the suggested changes to your DAG or %s to quit…", ansi.Green("Press Enter"), ansi.Red("^C"))
	fmt.Scanln()
	fmt.Println("\n" + fakeResponse2)

	i, _ := input.Confirm(
		fmt.Sprintf("\nWould you like to apply these changes to the file %s", filepath.Base(dagFilePath)))

	if !i {
		return errors.New("\nfix the issues outlined above and try again")
	}
	fileutil.WriteStringToFile(dagFilePath, correctedDAG)
	fmt.Println("\nChanges applied!")
	return nil
}
