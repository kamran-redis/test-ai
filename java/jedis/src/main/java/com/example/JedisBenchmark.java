package com.example;

import org.apache.commons.cli.*;
import redis.clients.jedis.Jedis;
import redis.clients.jedis.JedisPool;
import redis.clients.jedis.JedisPoolConfig;
import redis.clients.jedis.exceptions.JedisException;

import java.util.ArrayList;
import java.util.List;
import java.util.concurrent.*;
import java.util.concurrent.atomic.AtomicLong;

public class JedisBenchmark {

    public static void main(String[] args) {
        Options options = new Options();

        options.addOption("h", "host", true, "Redis host (default: localhost)");
        options.addOption("p", "port", true, "Redis port (default: 6379)");
        options.addOption("t", "threads", true, "Number of threads (default: 4)");
        options.addOption("n", "operations", true, "Number of operations per thread (default: 10000)");
        options.addOption("pw", "password", true, "Redis password (optional)");

        CommandLineParser parser = new DefaultParser();
        HelpFormatter formatter = new HelpFormatter();
        CommandLine cmd;

        String host = "localhost";
        int port = 6379;
        int numThreads = 4;
        int numOperationsPerThread = 10000;
        String password = null;

        try {
            cmd = parser.parse(options, args);
            if (cmd.hasOption("host")) host = cmd.getOptionValue("host");
            if (cmd.hasOption("port")) port = Integer.parseInt(cmd.getOptionValue("port"));
            if (cmd.hasOption("threads")) numThreads = Integer.parseInt(cmd.getOptionValue("threads"));
            if (cmd.hasOption("operations")) numOperationsPerThread = Integer.parseInt(cmd.getOptionValue("operations"));
            if (cmd.hasOption("password")) password = cmd.getOptionValue("password");
        } catch (ParseException e) {
            System.err.println("Error parsing command line arguments: " + e.getMessage());
            formatter.printHelp("JedisBenchmark", options);
            System.exit(1);
            return;
        }

        System.out.println("Configuration:");
        System.out.println("  Host: " + host);
        System.out.println("  Port: " + port);
        System.out.println("  Threads: " + numThreads);
        System.out.println("  Operations per thread: " + numOperationsPerThread);
        System.out.println("  Total operations: " + (long) numThreads * numOperationsPerThread);
        if (password != null) {
            System.out.println("  Password provided.");
        }
        System.out.println();

        JedisPoolConfig poolConfig = new JedisPoolConfig();
        poolConfig.setMaxTotal(numThreads + 5); // Max connections
        poolConfig.setMaxIdle(numThreads);      // Max idle connections
        poolConfig.setMinIdle(1);               // Min idle connections
        JedisPool jedisPool;
        if (password != null && !password.isEmpty()) {
            jedisPool = new JedisPool(poolConfig, host, port, 2000, password);
        } else {
            jedisPool = new JedisPool(poolConfig, host, port, 2000);
        }


        // Test connection
        try (Jedis jedis = jedisPool.getResource()) {
            System.out.println("Pinging Redis server...");
            String pingResult = jedis.ping();
            System.out.println("Redis PING response: " + pingResult);
            if (!"PONG".equalsIgnoreCase(pingResult)) {
                 System.err.println("Failed to connect to Redis. PING did not return PONG.");
                 jedisPool.close();
                 System.exit(1);
                 return;
            }
        } catch (JedisException e) {
            System.err.println("Could not connect to Redis: " + e.getMessage());
            e.printStackTrace();
            jedisPool.close();
            System.exit(1);
            return;
        }


        ExecutorService executor = Executors.newFixedThreadPool(numThreads);
        List<Future<BenchmarkResult>> futures = new ArrayList<>();
        AtomicLong totalSuccessfulOps = new AtomicLong(0);
        AtomicLong totalLatencyNanos = new AtomicLong(0);
        final int nos = numOperationsPerThread;
        long startTimeMillis = System.currentTimeMillis();

        for (int i = 0; i < numThreads; i++) {
            final int threadId = i;
            Callable<BenchmarkResult> task = () -> {
                long threadOps = 0;
                long threadLatencySumNanos = 0;
                try (Jedis jedis = jedisPool.getResource()) {
                    for (int j = 0; j < nos; j++) {
                        String key = "thread-" + threadId + "-op-" + j;
                        String value = "value-" + j;

                        long opStartTimeNanos = System.nanoTime();
                        jedis.set(key, value);
                        String retrievedValue = jedis.get(key);
                        long opEndTimeNanos = System.nanoTime();

                        if (value.equals(retrievedValue)) {
                            threadOps++;
                            threadLatencySumNanos += (opEndTimeNanos - opStartTimeNanos);
                        } else {
                             // System.err.println("Data mismatch for key: " + key);
                        }
                    }
                } catch (JedisException e) {
                    System.err.println("JedisException in thread " + threadId + ": " + e.getMessage());
                    // Optionally log or handle more gracefully
                }
                return new BenchmarkResult(threadOps, threadLatencySumNanos);
            };
            futures.add(executor.submit(task));
        }

        for (Future<BenchmarkResult> future : futures) {
            try {
                BenchmarkResult result = future.get();
                totalSuccessfulOps.addAndGet(result.successfulOperations);
                totalLatencyNanos.addAndGet(result.totalLatencyNanos);
            } catch (InterruptedException | ExecutionException e) {
                System.err.println("Error executing task: " + e.getMessage());
                e.printStackTrace();
            }
        }

        long endTimeMillis = System.currentTimeMillis();
        long totalTimeMillis = endTimeMillis - startTimeMillis;

        executor.shutdown();
        try {
            if (!executor.awaitTermination(5, TimeUnit.SECONDS)) {
                executor.shutdownNow();
            }
        } catch (InterruptedException e) {
            executor.shutdownNow();
            Thread.currentThread().interrupt();
        }

        jedisPool.close();

        double totalTimeSeconds = totalTimeMillis / 1000.0;
        double opsPerSecond = (totalTimeSeconds > 0) ? (totalSuccessfulOps.get() / totalTimeSeconds) : 0;
        double avgLatencyMillis = (totalSuccessfulOps.get() > 0) ? (totalLatencyNanos.get() / (double) totalSuccessfulOps.get()) / 1_000_000.0 : 0;

        System.out.println("\n--- Benchmark Results ---");
        System.out.printf("Total operations performed: %d\n", totalSuccessfulOps.get());
        System.out.printf("Total time taken: %.3f seconds\n", totalTimeSeconds);
        System.out.printf("Operations Per Second (OPS): %.2f\n", opsPerSecond);
        System.out.printf("Average latency per operation: %.4f ms\n", avgLatencyMillis);
        System.out.printf("Number of threads: %d\n", numThreads);
        System.out.println("------------------------");
    }

    private static class BenchmarkResult {
        final long successfulOperations;
        final long totalLatencyNanos;

        BenchmarkResult(long successfulOperations, long totalLatencyNanos) {
            this.successfulOperations = successfulOperations;
            this.totalLatencyNanos = totalLatencyNanos;
        }
    }
}
