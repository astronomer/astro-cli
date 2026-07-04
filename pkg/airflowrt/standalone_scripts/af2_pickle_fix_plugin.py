from airflow.executors.local_executor import LocalWorker, QueuedLocalWorker

QueuedLocalWorker.do_work.__name__ = "do_work"
QueuedLocalWorker.do_work.__qualname__ = "QueuedLocalWorker.do_work"
LocalWorker.do_work.__name__ = "do_work"
LocalWorker.do_work.__qualname__ = "LocalWorker.do_work"
