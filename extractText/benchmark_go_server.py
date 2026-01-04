import requests
import time
import statistics
from concurrent.futures import ThreadPoolExecutor, as_completed
import json

GO_SERVER_URL = "http://localhost:4567"
BATCH_SIZE = 8
MAX_SAMPLE_LENGTH = 4096

def fetch_batch():
    start_time = time.time()
    try:
        response = requests.post(
            f"{GO_SERVER_URL}/get-batch",
            json={
                "batch_size": BATCH_SIZE,
                "max_sample_length": MAX_SAMPLE_LENGTH
            },
            timeout=30
        )
        elapsed = time.time() - start_time
        
        if response.status_code == 200:
            data = response.json()
            return {
                'success': True,
                'elapsed': elapsed,
                'total_tokens': data['total_tokens'],
                'samples': len(data['samples'])
            }
        else:
            return {
                'success': False,
                'elapsed': elapsed,
                'error': f"Status {response.status_code}"
            }
    except Exception as e:
        elapsed = time.time() - start_time
        return {
            'success': False,
            'elapsed': elapsed,
            'error': str(e)
        }

def get_cache_stats():
    try:
        response = requests.get(f"{GO_SERVER_URL}/cache-stats", timeout=5)
        if response.status_code == 200:
            return response.json()
    except:
        pass
    return None

def benchmark_sequential(num_requests):
    print(f"\n{'='*60}")
    print(f"Sequential Benchmark - {num_requests} requests")
    print(f"{'='*60}")
    
    results = []
    total_samples = 0
    total_tokens = 0
    
    start_time = time.time()
    
    for i in range(num_requests):
        result = fetch_batch()
        results.append(result)
        
        if result['success']:
            total_samples += result['samples']
            total_tokens += result['total_tokens']
            
        if (i + 1) % 10 == 0:
            print(f"Progress: {i + 1}/{num_requests}")
    
    total_time = time.time() - start_time
    
    successful = [r for r in results if r['success']]
    failed = [r for r in results if not r['success']]
    
    if successful:
        latencies = [r['elapsed'] for r in successful]
        
        print(f"\nResults:")
        print(f"Total time: {total_time:.2f}s")
        print(f"Successful requests: {len(successful)}")
        print(f"Failed requests: {len(failed)}")
        print(f"Total samples: {total_samples}")
        print(f"Total tokens: {total_tokens:,}")
        print(f"\nLatency:")
        print(f"  Min: {min(latencies)*1000:.2f}ms")
        print(f"  Max: {max(latencies)*1000:.2f}ms")
        print(f"  Mean: {statistics.mean(latencies)*1000:.2f}ms")
        print(f"  Median: {statistics.median(latencies)*1000:.2f}ms")
        print(f"\nThroughput:")
        print(f"  Requests/sec: {len(successful) / total_time:.2f}")
        print(f"  Samples/sec: {total_samples / total_time:.2f}")
        print(f"  Tokens/sec: {total_tokens / total_time:,.2f}")
    
    cache_stats = get_cache_stats()
    if cache_stats:
        print(f"\nCache Stats:")
        print(f"  Size: {cache_stats['cache_size']}")
        print(f"  Capacity: {cache_stats['cache_capacity']}")
        print(f"  Utilization: {cache_stats['cache_utilization']:.1f}%")
        print(f"  Refilling: {cache_stats['is_refilling']}")

def benchmark_concurrent(num_requests, num_workers):
    print(f"\n{'='*60}")
    print(f"Concurrent Benchmark - {num_requests} requests, {num_workers} workers")
    print(f"{'='*60}")
    
    results = []
    total_samples = 0
    total_tokens = 0
    
    start_time = time.time()
    
    with ThreadPoolExecutor(max_workers=num_workers) as executor:
        futures = [executor.submit(fetch_batch) for _ in range(num_requests)]
        
        for i, future in enumerate(as_completed(futures)):
            result = future.result()
            results.append(result)
            
            if result['success']:
                total_samples += result['samples']
                total_tokens += result['total_tokens']
            
            if (i + 1) % 10 == 0:
                print(f"Progress: {i + 1}/{num_requests}")
    
    total_time = time.time() - start_time
    
    successful = [r for r in results if r['success']]
    failed = [r for r in results if not r['success']]
    
    if successful:
        latencies = [r['elapsed'] for r in successful]
        
        print(f"\nResults:")
        print(f"Total time: {total_time:.2f}s")
        print(f"Successful requests: {len(successful)}")
        print(f"Failed requests: {len(failed)}")
        print(f"Total samples: {total_samples}")
        print(f"Total tokens: {total_tokens:,}")
        print(f"\nLatency:")
        print(f"  Min: {min(latencies)*1000:.2f}ms")
        print(f"  Max: {max(latencies)*1000:.2f}ms")
        print(f"  Mean: {statistics.mean(latencies)*1000:.2f}ms")
        print(f"  Median: {statistics.median(latencies)*1000:.2f}ms")
        print(f"\nThroughput:")
        print(f"  Requests/sec: {len(successful) / total_time:.2f}")
        print(f"  Samples/sec: {total_samples / total_time:.2f}")
        print(f"  Tokens/sec: {total_tokens / total_time:,.2f}")
    
    cache_stats = get_cache_stats()
    if cache_stats:
        print(f"\nCache Stats:")
        print(f"  Size: {cache_stats['cache_size']}")
        print(f"  Capacity: {cache_stats['cache_capacity']}")
        print(f"  Utilization: {cache_stats['cache_utilization']:.1f}%")
        print(f"  Refilling: {cache_stats['is_refilling']}")

def benchmark_sustained(duration_seconds, workers=1):
    print(f"\n{'='*60}")
    print(f"Sustained Load Test - {duration_seconds}s duration, {workers} workers")
    print(f"{'='*60}")
    
    results = []
    total_samples = 0
    total_tokens = 0
    
    start_time = time.time()
    end_time = start_time + duration_seconds
    
    request_count = 0
    
    with ThreadPoolExecutor(max_workers=workers) as executor:
        futures = []
        
        while time.time() < end_time:
            futures.append(executor.submit(fetch_batch))
            request_count += 1
            
            if workers == 1:
                time.sleep(0.01)
        
        print(f"Submitted {request_count} requests, waiting for completion...")
        
        for i, future in enumerate(as_completed(futures)):
            result = future.result()
            results.append(result)
            
            if result['success']:
                total_samples += result['samples']
                total_tokens += result['total_tokens']
            
            if (i + 1) % 50 == 0:
                print(f"Completed: {i + 1}/{request_count}")
    
    total_time = time.time() - start_time
    
    successful = [r for r in results if r['success']]
    failed = [r for r in results if not r['success']]
    
    if successful:
        latencies = [r['elapsed'] for r in successful]
        
        print(f"\nResults:")
        print(f"Total time: {total_time:.2f}s")
        print(f"Successful requests: {len(successful)}")
        print(f"Failed requests: {len(failed)}")
        print(f"Total samples: {total_samples}")
        print(f"Total tokens: {total_tokens:,}")
        print(f"\nLatency:")
        print(f"  Min: {min(latencies)*1000:.2f}ms")
        print(f"  Max: {max(latencies)*1000:.2f}ms")
        print(f"  Mean: {statistics.mean(latencies)*1000:.2f}ms")
        print(f"  Median: {statistics.median(latencies)*1000:.2f}ms")
        if len(latencies) > 1:
            print(f"  StdDev: {statistics.stdev(latencies)*1000:.2f}ms")
        print(f"\nThroughput:")
        print(f"  Requests/sec: {len(successful) / total_time:.2f}")
        print(f"  Samples/sec: {total_samples / total_time:.2f}")
        print(f"  Tokens/sec: {total_tokens / total_time:,.2f}")
    
    cache_stats = get_cache_stats()
    if cache_stats:
        print(f"\nCache Stats:")
        print(f"  Size: {cache_stats['cache_size']}")
        print(f"  Capacity: {cache_stats['cache_capacity']}")
        print(f"  Utilization: {cache_stats['cache_utilization']:.1f}%")
        print(f"  Refilling: {cache_stats['is_refilling']}")

def main():
    print("Go Server Benchmark Tool")
    print(f"Server: {GO_SERVER_URL}")
    print(f"Batch size: {BATCH_SIZE}")
    print(f"Max sample length: {MAX_SAMPLE_LENGTH}")
    
    try:
        health_response = requests.get(f"{GO_SERVER_URL}/health", timeout=5)
        if health_response.status_code != 200:
            print("\nError: Go server is not healthy!")
            return
        print("Server is healthy!")
    except Exception as e:
        print(f"\nError: Cannot connect to Go server: {e}")
        return
    
    cache_stats = get_cache_stats()
    if cache_stats:
        print(f"\nInitial Cache Stats:")
        print(f"  Size: {cache_stats['cache_size']}")
        print(f"  Capacity: {cache_stats['cache_capacity']}")
        print(f"  Utilization: {cache_stats['cache_utilization']:.1f}%")
    
    benchmark_sequential(100)
    
    print("\n" + "="*60)
    print("Waiting 5 seconds for cache to refill...")
    print("="*60)
    time.sleep(5)
    
    benchmark_concurrent(100, num_workers=4)
    
    print("\n" + "="*60)
    print("Waiting 5 seconds for cache to refill...")
    print("="*60)
    time.sleep(5)
    
    benchmark_concurrent(100, num_workers=8)
    
    print("\n" + "="*60)
    print("Waiting 5 seconds for cache to refill...")
    print("="*60)
    time.sleep(5)
    
    benchmark_sustained(30, workers=1)
    
    print("\n" + "="*60)
    print("Benchmark Complete!")
    print("="*60)

if __name__ == "__main__":
    main()
