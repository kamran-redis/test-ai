# .NET Redis Performance Test Application

## Original Issue Description

create a sample .Net application connecting to redis , adding parallelism and measuring operations per second and latency. Give the ability to provide parameters using command line create the code in a subdirectory dotnet.

Also make this prompt part of Readme

## Description

This application is a .NET console tool designed to connect to a Redis instance, perform operations with a configurable level of parallelism, and measure performance metrics such as Operations Per Second (OPS) and average latency (though current version focuses on OPS for duration-based tests).

It allows users to specify parameters via the command line, including:
- Redis host
- Redis port
- Number of parallel tasks
- Duration of the test in seconds

## Prerequisites

- .NET SDK (8.0 or later recommended)
- Access to a running Redis instance

## Building the Application

1. Navigate to the `dotnet` directory:
   ```bash
   cd dotnet
   ```
2. Build the project:
   ```bash
   dotnet build -c Release
   ```

## Running the Application

After building, you can run the application from within the `dotnet` directory.

### Basic Usage (with default parameters):

```bash
dotnet run --project . -c Release
```

This will connect to Redis at `localhost:6379`, run with 100 parallel tasks for 10 seconds.

### Custom Parameters:

You can specify parameters using command-line options:

```bash
dotnet run --project . -c Release -- --rhost <your_redis_host> --rport <your_redis_port> --ntasks <number_of_tasks> --duration <test_duration_seconds>
```

**Example:**

```bash
dotnet run --project . -c Release -- --rhost myredisserver --rport 6380 --ntasks 50 --duration 30
```

This will connect to `myredisserver:6380`, use 50 parallel tasks, and run the test for 30 seconds.

### Available Options:

- `--rhost`: (Default: "localhost") The hostname or IP address of the Redis server.
- `--rport`: (Default: 6379) The port number of the Redis server.
- `--ntasks`: (Default: 100) The number of parallel tasks to execute Redis operations.
- `--duration`: (Default: 10) The duration of the test in seconds.
- `--help`: Displays the help screen with all available options.

```
