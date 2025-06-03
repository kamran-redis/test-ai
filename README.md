# Java Redis Benchmark

## Original Prompt

create a sample java application connecting to redis , adding parallism either using threads or executor framewwork and measuring operations per second and latency. Give the ability to provide parameters using command line Use gradle for build, create the code in a subdirectory java/jedis.

Also make this prompt part of Readme

---
# test-ai

## Building and Running the Benchmark

This project uses Gradle to manage dependencies and build the application.

### Prerequisites

*   Java Development Kit (JDK) 8 or higher installed.
*   A running Redis instance.

### Building

1.  Navigate to the `java/jedis` directory:
    ```bash
    cd java/jedis
    ```
2.  Build the application using Gradle. On Linux/macOS:
    ```bash
    ./gradlew build
    ```
    On Windows:
    ```bash
    gradlew.bat build
    ```
    This command will download dependencies and compile the source code. It will also create a runnable JAR.

### Running

After a successful build, you can run the application. The runnable JAR will be located in `java/jedis/build/libs/`.

To run the application with default settings (connects to `localhost:6379`, 4 threads, 10000 operations per thread):

```bash
java -jar build/libs/jedis-all.jar
```

Or, if your `build.gradle` produces a plain jar without dependencies bundled:
```bash
./gradlew run
```
(Or `gradlew.bat run` on Windows)


You can specify command-line arguments to change the behavior:

*   `--host <hostname>`: Redis server hostname (e.g., `127.0.0.1`).
*   `--port <port_number>`: Redis server port (e.g., `6380`).
*   `--threads <num_threads>`: Number of concurrent threads to use (e.g., `8`).
*   `--operations <num_ops_per_thread>`: Number of SET/GET operations each thread will perform (e.g., `50000`).
*   `--password <password>`: Password for your Redis instance, if required.

**Example with custom parameters:**

```bash
java -jar build/libs/jedis-all.jar --host my.redis.server --port 6379 --threads 8 --operations 25000 --password "yourSecurePassword"
```

Or using the Gradle `run` task (arguments are passed after `--args`):
```bash
./gradlew run --args="--host my.redis.server --port 6379 --threads 8 --operations 25000 --password yourSecurePassword"
```

The application will print the benchmark results, including total operations, operations per second (OPS), and average latency.

**Note:** The `build.gradle` includes the `application` plugin and shadow JAR configuration (or equivalent fat JAR) to package dependencies, so `java -jar build/libs/jedis-all.jar` (the JAR name might vary slightly based on exact Gradle config, e.g. `jedis.jar` or `jedis-VERSION-all.jar`) should work. If you encounter issues with finding the main class or dependencies, ensure your `build.gradle` correctly configures the `mainClassName` and packages the dependencies into the JAR. The provided `build.gradle` is set up to create a fat JAR.