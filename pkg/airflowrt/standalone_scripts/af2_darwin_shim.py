#!/usr/bin/env python3
"""AF2 standalone shim for macOS — avoids gunicorn fork crash."""
import os, signal, subprocess, sys, threading, time


def _stream(proc, prefix):
    for line in iter(proc.stdout.readline, b""):
        sys.stdout.buffer.write(("{} | ".format(prefix)).encode() + line)
        sys.stdout.buffer.flush()
    proc.stdout.close()


def main():
    af = [sys.executable, "-m", "airflow"]

    # DB migrations
    print("standalone | Running DB migrations...", flush=True)
    subprocess.run(af + ["db", "migrate"], check=True)

    # Idempotent admin user creation
    result = subprocess.run(
        af
        + [
            "users",
            "create",
            "--role",
            "Admin",
            "--username",
            "admin",
            "--password",
            "admin",
            "--firstname",
            "Admin",
            "--lastname",
            "User",
            "--email",
            "admin@example.com",
        ],
        capture_output=True,
        text=True,
    )
    if result.returncode == 0:
        print("standalone | Created admin user.  Login: admin / admin", flush=True)
    elif "already exists" in (result.stdout + result.stderr):
        print(
            "standalone | Admin user already exists.  Login: admin / admin", flush=True
        )
    else:
        print(
            "standalone | WARNING: failed to create admin user (exit {})".format(
                result.returncode
            ),
            flush=True,
        )
        if result.stderr:
            for line in result.stderr.strip().splitlines():
                print("standalone |   " + line, flush=True)

    # Start components — webserver uses Flask dev server (--debug) to
    # avoid gunicorn's fork which crashes on macOS.
    procs = {}
    for name, extra in [
        ("scheduler", []),
        ("webserver", ["--debug"]),
        ("triggerer", []),
    ]:
        procs[name] = subprocess.Popen(
            af + [name] + extra,
            stdout=subprocess.PIPE,
            stderr=subprocess.STDOUT,
        )

    for name, proc in procs.items():
        t = threading.Thread(target=_stream, args=(proc, name), daemon=True)
        t.start()

    def shutdown(sig=None, frame=None):
        for proc in procs.values():
            try:
                proc.terminate()
            except OSError:
                pass
        for proc in procs.values():
            try:
                proc.wait(timeout=10)
            except Exception:
                proc.kill()
        sys.exit(0)

    signal.signal(signal.SIGTERM, shutdown)
    signal.signal(signal.SIGINT, shutdown)

    # Block until any component exits, then tear down everything.
    while all(p.poll() is None for p in procs.values()):
        time.sleep(1)

    for name, proc in procs.items():
        rc = proc.poll()
        if rc is not None:
            print("standalone | {} exited with code {}".format(name, rc), flush=True)
    shutdown()


if __name__ == "__main__":
    main()
