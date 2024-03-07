package askastro

var (
	dagReviewPrompt = `Review the DAG and identify any potential issues that should be corrected. In particular, flag anything related to the following:

	1. idempotency issues
	2. non-atomic tasks
	3. repeated code
	4. top level code that would be parsed in an inefficient manner
	5. task dependency inconsistencies
	6. non-static start_date
	7. retries not set
	
	Format the reply as a JSON object, with boolean key/value pairs for each of the five issues mentioned above, and further suggestions provided in a "context" object. Finally, a "corrected_dag" object should include a DAG that addresses any issues encountered. Make sure only DAG code is in "corrected_dag" section. Below is an example of the format required. Both "context" and "corrected_dag" must be parseble by a JSON parser. This means in those obejects use "//n" and "//t" for new line and tab:
	
	{
	  "health_check_values":
	  {
		"idempotency": true,
		"atomic_tasks": false,
		"unique_code": true,
		"no_top_level_code": false,
		"task_dependency_consistency": true,
		"static_start_date": true,
		"retries_set": true
	  },
	  "context": "Here are some potential issues with your DAG:\\n\\nIdempotency issues: Your DAG is not idempotent because the SQL insert query will insert new records every time the DAG is run. If the DAG is rerun for the same execution date, it will duplicate the records in the purchase_order table. A better approach would be to design your SQL query to handle duplicates, for example by using the INSERT ... ON CONFLICT DO NOTHING syntax in PostgreSQL.\\n\\nNon-atomic tasks: Your DAG seems to have a single task that inserts all records into the purchase_order table. If there's a failure partway through, there's no way to rerun just the failed inserts. Consider breaking this up into separate tasks for each insert, or using a single task that can handle partial failures.\n\nRepeated code: The code to generate the SQL queries is repeated for each record in the grocery_list table. This could be simplified by using a single SQL query with a SELECT ... INTO clause to insert the records directly from one table to another.\n\nLack of fields, variables or macros: You're not using any Airflow template fields, variables or macros in your DAG. These could be used to make your DAG more flexible and reusable. For example, you could use a template field for the table name, or a variable for the database connection ID.\n\nTop level code: You're running code to fetch records from the grocery_list table and generate SQL queries at the top level of your DAG file. This code will be run every time the DAG is parsed, which can cause performance issues. It's better to move this code into a function that's called from within a task. This way, the code is only run when the task is executed, not when the DAG is parsed.",
	  "corrected_dag": "<dag-code>"
	}
	
	Here is the DAG to evaluate:
	
	%s`

	dagReviewPrompt2 = `Review the DAG and identify any potential issues that should be corrected. In particular, flag anything related to the following:

	1. idempotency issues
	2. non-atomic tasks
	3. repeated code
	4. top level code that would be parsed in an inefficient manner
	5. task dependency inconsistencies
	6. non-static start_date
	7. retries not set

	Also provide a DAG that addresses the issues encountered. Here is an example output:

	-----------example output below-----------

	Review Results:
	Idempotency: x
	Atomic Tasks: x
	Unique Code: ✔
	No Top Level Code: x
	Task Dependency Consistency: ✔
	Static Start Date: ✔
	Retries Set: ✔

	Here are some potential issues with your DAG:

	Idempotency issues: Your DAG is not idempotent because the SQL insert query will
	insert new records every time the DAG is run. If the DAG is rerun for the same
	execution date, it will duplicate the records in the purchase_order table. A
	better approach would be to design your SQL query to handle duplicates, for
	example by using the INSERT ... ON CONFLICT DO NOTHING syntax in PostgreSQL.

	Non-atomic tasks: Your DAG seems to have a single task that inserts all records
	into the purchase_order table. If there's a failure partway through, there's no
	way to rerun just the failed inserts. Consider breaking this up into separate
	tasks for each insert, or using a single task that can handle partial failures.

	Top level code: You're running code to fetch records from the grocery_list table
	and generate SQL queries at the top level of your DAG file. This code will be
	run every time the DAG is parsed, which can cause performance issues. It's
	better to move this code into a function that's called from within a task. This
	way, the code is only run when the task is executed, not when the DAG is parsed.

	Retries not set: You have not set any retries for your tasks. It's a good
	practice to set retries for tasks that might fail transiently.

	Suggested DAG:

	from airflow.decorators import dag
	from airflow.providers.postgres.operators.postgres import PostgresOperator
	from airflow.providers.postgres.hooks.postgres import PostgresHook
	from pendulum import datetime

	@dag(start_date=datetime(2023, 1, 1), max_active_runs=3, schedule='@daily', catchup=False)
	def bad_practices_dag_1():

		def generate_sql_queries():
			hook = PostgresHook('database_conn')
			results = hook.get_records('SELECT * FROM grocery_list;')
			sql_queries = []
			for result in results:
				grocery = result[0]
				amount = result[1]
				sql_query = f"INSERT INTO purchase_order VALUES ('{grocery}', {amount}) ON CONFLICT DO NOTHING;"
				sql_queries.append(sql_query)
			return sql_queries

		insert_into_purchase_order_postgres = PostgresOperator.partial(
			task_id='insert_into_purchase_order_postgres',
			postgres_conn_id='postgres_default',
			retries=3
		).expand(sql=generate_sql_queries())

	bad_practices_dag_1()

	-----------example output above-----------

	Here is the DAG to evaluate:
	
	`

	fakeResponse = "\n{\n  \"health_check_values\":\n  {\n    \"idempotency\": false,\n    \"atomic_tasks\": false,\n    \"unique_code\": true,\n    \"no_top_level_code\": false,\n    \"task_dependency_consistency\": true,\n    \"static_start_date\": true,\n    \"retries_set\": true\n  },\n  \"context\": \"Here are some potential issues with your DAG:\\n\\nIdempotency issues: Your DAG is not idempotent because the SQL insert query will insert new records every time the DAG is run. If the DAG is rerun for the same execution date, it will duplicate the records in the purchase_order table. A better approach would be to design your SQL query to handle duplicates, for example by using the INSERT ... ON CONFLICT DO NOTHING syntax in PostgreSQL.\\n\\nNon-atomic tasks: Your DAG seems to have a single task that inserts all records into the purchase_order table. If there's a failure partway through, there's no way to rerun just the failed inserts. Consider breaking this up into separate tasks for each insert, or using a single task that can handle partial failures.\\n\\nTop level code: You're running code to fetch records from the grocery_list table and generate SQL queries at the top level of your DAG file. This code will be run every time the DAG is parsed, which can cause performance issues. It's better to move this code into a function that's called from within a task. This way, the code is only run when the task is executed, not when the DAG is parsed.\",\n  \"corrected_dag\": \"\\nfrom airflow.decoratorsimport\\tDAG\\nfrom airflow.operators.dummy_operator\\timport\\tDummyOperator\\nfrom airflow.operators.python_operator\\timport\\tPythonOperator\\nfromdatetime\\timport\\tdatetime\\n\\ndef\\tprint_hello():\\ntreturn\\t'Hellotworld!'\\n\\ndagt=tDAG(\\n\\t'hello_world',\\n\\tdescriptio\\n='SimplettutorialtDAG',\\n\\tschedule_interval='0t12t*t*t*',\\n\\tstart_date=datetime(2017,t3,t20),\\n\\tcatchup=False\\n)\\n\\ndummy_operator\\t=\\tDummyOperator(task_id='dummy_task',\\tretries=3,\\tdag=dag)\\n\\nhello_operatort=tPytho\\nOperator(\\n\\ttask_id='hello_task',\\n\\tpython_callable=print_hello,\\n\\tdag=dag\\n)\\n\\ndummy_operatort>>thello_operator\\n\"\n}\n"

	fakeResponse1 = `Here are some potential issues with your DAG:

Idempotency issues: Your DAG is not idempotent because the SQL insert query will
insert new records every time the DAG is run. If the DAG is rerun for the same
execution date, it will duplicate the records in the purchase_order table. A
better approach would be to design your SQL query to handle duplicates, for
example by using the INSERT ... ON CONFLICT DO NOTHING syntax in PostgreSQL.

Non-atomic tasks: Your DAG seems to have a single task that inserts all records
into the purchase_order table. If there's a failure partway through, there's no
way to rerun just the failed inserts. Consider breaking this up into separate
tasks for each insert, or using a single task that can handle partial failures.

Top level code: You're running code to fetch records from the grocery_list table
and generate SQL queries at the top level of your DAG file. This code will be
run every time the DAG is parsed, which can cause performance issues. It's
better to move this code into a function that's called from within a task. This
way, the code is only run when the task is executed, not when the DAG is parsed.

Retries not set: You have not set any retries for your tasks. It's a good
practice to set retries for tasks that might fail transiently.`

	fakeResponse2 = `from airflow.decorators import dag
from airflow.providers.postgres.operators.postgres import PostgresOperator
from airflow.providers.postgres.hooks.postgres import PostgresHook
from pendulum import datetime

@dag(start_date=datetime(2023, 1, 1), max_active_runs=3, schedule='@daily', catchup=False)
def bad_practices_dag_1():

    def generate_sql_queries():
        hook = PostgresHook('database_conn')
        results = hook.get_records('SELECT * FROM grocery_list;')
        sql_queries = []
        for result in results:
            grocery = result[0]
            amount = result[1]
            sql_query = f"INSERT INTO purchase_order VALUES ('{grocery}', {amount}) ON CONFLICT DO NOTHING;"
            sql_queries.append(sql_query)
        return sql_queries

    insert_into_purchase_order_postgres = PostgresOperator.partial(
        task_id='insert_into_purchase_order_postgres',
        postgres_conn_id='postgres_default',
        retries=3
    ).expand(sql=generate_sql_queries())

bad_practices_dag_1()`
)
