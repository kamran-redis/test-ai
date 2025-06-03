import argparse
import redis
import threading
import time
import statistics
import os # For generating unique keys if needed

def run_worker(worker_id, operations, host, port, password, results_list):
    successful_ops = 0
    latencies = []

    try:
        r = redis.Redis(host=host, port=port, password=password, db=0, decode_responses=True)
        r.ping()
    except redis.exceptions.ConnectionError as e:
        print(f"Worker {worker_id}: Could not connect to Redis: {e}")
        results_list.append({'id': worker_id, 'ops': 0, 'latencies': []})
        return

    for i in range(operations):
        key = f"worker-{worker_id}-op-{i}"
        # Add some randomness to value to prevent caching effects if any
        value = f"value-{i}-{os.urandom(4).hex()}"

        try:
            op_start_time = time.perf_counter()
            r.set(key, value)
            retrieved_value = r.get(key)
            op_end_time = time.perf_counter()

            if retrieved_value == value:
                successful_ops += 1
                latencies.append((op_end_time - op_start_time) * 1000) # Store latency in ms
            # else:
                # print(f"Worker {worker_id}: Data mismatch for key {key}")
        except redis.exceptions.RedisError as e:
            # print(f"Worker {worker_id}: Redis error for key {key}: {e}")
            pass

    results_list.append({'id': worker_id, 'ops': successful_ops, 'latencies': latencies})

def main():
    parser = argparse.ArgumentParser(description="Python Redis Benchmark Tool")
    parser.add_argument("--host", default="localhost", help="Redis host")
    parser.add_argument("--port", type=int, default=6379, help="Redis port")
    parser.add_argument("--workers", type=int, default=4, help="Number of worker threads")
    parser.add_argument("--operations", type=int, default=10000, help="Number of operations per worker")
    parser.add_argument("--password", default=None, help="Redis password (optional)")

    args = parser.parse_args()

    print("Python Redis Benchmark")
    print("----------------------")
    print(f"Configuration:")
    print(f"  Host: {args.host}")
    print(f"  Port: {args.port}")
    print(f"  Workers (threads): {args.workers}")
    print(f"  Operations per worker: {args.operations}")
    print(f"  Total expected operations: {args.workers * args.operations}")
    if args.password:
        print("  Password provided.")
    print("----------------------\n")

    try:
        r_test = redis.Redis(host=args.host, port=args.port, password=args.password, db=0)
        r_test.ping()
        print("Successfully connected to Redis (main test connection).\n")
    except redis.exceptions.ConnectionError as e:
        print(f"Could not connect to Redis with main client: {e}")
        # Depending on strictness, might exit here or let workers try.
        # For this benchmark, we'll let workers attempt their own connections.

    threads = []
    worker_results = []

    overall_start_time = time.perf_counter()

    for i in range(args.workers):
        # Each thread gets its own list to append to, or a shared list with locking
        # For simplicity, passing the main list; appends are thread-safe on list objects in CPython
        thread = threading.Thread(target=run_worker, args=(i, args.operations, args.host, args.port, args.password, worker_results))
        threads.append(thread)
        thread.start()

    for thread in threads:
        thread.join()

    overall_end_time = time.perf_counter()
    total_duration_seconds = overall_end_time - overall_start_time

    total_successful_ops = 0
    all_latencies_ms = []

    for res in worker_results:
        total_successful_ops += res['ops']
        all_latencies_ms.extend(res['latencies'])

    ops_per_second = 0
    if total_duration_seconds > 0:
        ops_per_second = total_successful_ops / total_duration_seconds

    avg_latency_ms = 0
    min_latency_ms = 0
    max_latency_ms = 0
    # p95_latency_ms = 0
    # p99_latency_ms = 0

    if all_latencies_ms:
        avg_latency_ms = statistics.mean(all_latencies_ms)
        min_latency_ms = min(all_latencies_ms)
        max_latency_ms = max(all_latencies_ms)
        # Python 3.8+ for statistics.quantiles
        # if hasattr(statistics, 'quantiles'):
        #     qs = statistics.quantiles(all_latencies_ms, n=100)
        #     p95_latency_ms = qs[94] # Corresponds to 95th percentile
        #     p99_latency_ms = qs[98] # Corresponds to 99th percentile


    print("\n--- Benchmark Results ---")
    print(f"Total operations performed: {total_successful_ops}")
    print(f"Total time taken: {total_duration_seconds:.3f} seconds")
    print(f"Operations Per Second (OPS): {ops_per_second:.2f}")
    print(f"Average latency per operation: {avg_latency_ms:.4f} ms")
    print(f"Number of workers: {args.workers}")
    if all_latencies_ms:
        print(f"Min latency: {min_latency_ms:.4f} ms")
        print(f"Max latency: {max_latency_ms:.4f} ms")
        # if hasattr(statistics, 'quantiles'):
        #     print(f"P95 latency: {p95_latency_ms:.4f} ms")
        #     print(f"P99 latency: {p99_latency_ms:.4f} ms")
    print("------------------------")

if __name__ == "__main__":
    main()
