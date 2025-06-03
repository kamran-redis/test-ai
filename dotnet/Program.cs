using StackExchange.Redis;
using System;
using System.Collections.Generic;
using System.Threading.Tasks;
using System.Diagnostics;
using System.Threading;
using CommandLine; // Added for command line parsing

public class Options
{
    [Option('h', "host", Required = false, Default = "localhost", HelpText = "Redis server host.")]
    public string RedisHost { get; set; } = string.Empty; // Initialize to satisfy CS8618

    [Option('p', "port", Required = false, Default = 6379, HelpText = "Redis server port.")]
    public int RedisPort { get; set; }

    [Option('n', "numtasks", Required = false, Default = 100, HelpText = "Number of parallel tasks to run.")]
    public int NumTasks { get; set; }

    [Option('d', "duration", Required = false, Default = 10, HelpText = "Test duration in seconds.")]
    public int TestDurationSeconds { get; set; }
}

class Program
{
    static async Task Main(string[] args)
    {
        await Parser.Default.ParseArguments<Options>(args)
            .WithParsedAsync(RunOptionsAndReturnExitCode);
        // WithNotParsed is handled by default by CommandLineParser to show help
    }

    static async Task RunOptionsAndReturnExitCode(Options opts)
    {
        Console.WriteLine("Application Configuration:");
        Console.WriteLine($"  Redis Host: {opts.RedisHost}");
        Console.WriteLine($"  Redis Port: {opts.RedisPort}");
        Console.WriteLine($"  Number of Tasks: {opts.NumTasks}");
        Console.WriteLine($"  Test Duration: {opts.TestDurationSeconds} seconds");
        Console.WriteLine("---");

        string redisConnectionString = $"{opts.RedisHost}:{opts.RedisPort}";
        IDatabase? db = null;
        ConnectionMultiplexer? redis = null;

        try
        {
            Console.WriteLine($"\nConnecting to Redis at {redisConnectionString}...");
            redis = ConnectionMultiplexer.Connect(redisConnectionString);
            db = redis.GetDatabase();
            Console.WriteLine("Successfully connected to Redis.");

            Console.WriteLine($"\nStarting benchmark with {opts.NumTasks} parallel tasks for {opts.TestDurationSeconds} seconds...");

            long totalSuccessfulOperations = 0;
            var cancellationTokenSource = new CancellationTokenSource();
            var stopwatch = Stopwatch.StartNew();

            List<Task> tasks = new List<Task>();
            for (int i = 0; i < opts.NumTasks; i++)
            {
                int taskId = i; // Capture task ID for unique key generation
                tasks.Add(Task.Run(async () =>
                {
                    long taskOperations = 0;
                    // Use a unique key prefix per task to minimize contention if needed, or a common pool
                    // For simplicity, each task will use its own set of keys.
                    // This key generation strategy might lead to many keys if duration is long.
                    // For a benchmark focusing on raw OPS, this might be fine.
                    // Consider key rotation or fixed set of keys if memory is a concern for long tests.
                    int keyCounter = 0;
                    while (!cancellationTokenSource.Token.IsCancellationRequested)
                    {
                        string key = $"task{taskId}_key_{keyCounter++}";
                        string value = "some_value"; // Static value to reduce overhead
                        try
                        {
                            if (await db.StringSetAsync(key, value, TimeSpan.FromMinutes(opts.TestDurationSeconds > 1 ? opts.TestDurationSeconds : 2))) // Set expiry to avoid polluting Redis too much
                            {
                                Interlocked.Increment(ref totalSuccessfulOperations);
                                taskOperations++;
                            }
                        }
                        catch (RedisConnectionException) { /* Handle if needed, e.g. stop task */ return; }
                        catch (ObjectDisposedException) { /* Multiplexer closed */ return; }
                        catch (Exception) { /* Log or handle other op errors */ } // Potentially log and continue
                    }
                    // Console.WriteLine($"Task {taskId} completed {taskOperations} operations."); // Optional: per-task stats
                }, cancellationTokenSource.Token));
            }

            // Let the tasks run for the specified duration
            await Task.Delay(TimeSpan.FromSeconds(opts.TestDurationSeconds), cancellationTokenSource.Token)
                .ContinueWith(_ => { }); // Use ContinueWith to handle potential TaskCanceledException from Task.Delay if token is cancelled by other means (not applicable here but good practice)

            cancellationTokenSource.Cancel(); // Signal tasks to stop

            try
            {
                await Task.WhenAll(tasks.ToArray()); // Wait for all tasks to acknowledge cancellation and finish
            }
            catch (TaskCanceledException)
            {
                // Expected if tasks are still running when token is signaled from Task.Delay
                Console.WriteLine("Tasks correctly responded to cancellation.");
            }
            catch(Exception ex) // Catch other potential aggregate exceptions
            {
                 Console.WriteLine($"Exception during task completion: {ex.Message}");
            }


            stopwatch.Stop();

            double actualTestDurationSeconds = stopwatch.Elapsed.TotalSeconds;
            double operationsPerSecond = totalSuccessfulOperations / actualTestDurationSeconds;
            // Average latency is harder to calculate accurately here without per-operation timing,
            // especially as tasks run truly in parallel and might be delayed by client-side or network.
            // The previous calculation was for sequential operations within Parallel.For.
            // For now, we'll report total OPS.

            Console.WriteLine("\nBenchmark Summary:");
            Console.WriteLine($"  Target Test Duration: {opts.TestDurationSeconds} seconds");
            Console.WriteLine($"  Actual Test Duration: {actualTestDurationSeconds:F3} seconds");
            Console.WriteLine($"  Total Successful Operations: {totalSuccessfulOperations}");
            Console.WriteLine($"  Operations Per Second (OPS): {operationsPerSecond:F2}");

            // Cleanup: For a duration-based test with potentially millions of keys,
            // deleting them one by one is impractical and slow.
            // Strategies:
            // 1. Use a specific Redis database for testing and FLUSHDB (if permissible).
            // 2. Use keys with a common prefix and use SCAN + DEL in batches (still slow for large N).
            // 3. Set TTL on all keys during the test so they expire automatically. (Implemented above with StringSetAsync expiry)
            Console.WriteLine("\nNote: Keys set during this test were given a TTL and will expire automatically.");
            Console.WriteLine("No explicit cleanup of individual keys will be performed by this tool for duration-based tests.");

        }
        catch (RedisConnectionException ex)
        {
            Console.WriteLine($"\nRedis connection error: {ex.Message}");
            Console.WriteLine("Please ensure Redis server is running and accessible.");
        }
        catch (Exception ex)
        {
            Console.WriteLine($"\nAn unexpected error occurred: {ex.Message}");
        }
        finally
        {
            if (redis != null)
            {
                Console.WriteLine("\nClosing Redis connection...");
                redis.Close();
                Console.WriteLine("Redis connection closed.");
            }
        }
    }
}
