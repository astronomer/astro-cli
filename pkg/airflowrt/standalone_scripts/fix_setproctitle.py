import sys
import os

if sys.platform == "darwin":
    if hasattr(os, "fork"):
        os._real_fork = os.fork
        del os.fork

    try:
        import setproctitle

        setproctitle.setproctitle = lambda *a, **kw: None
        setproctitle.setthreadtitle = lambda *a, **kw: None
    except ImportError:
        pass
