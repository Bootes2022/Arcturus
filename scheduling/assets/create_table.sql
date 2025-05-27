
CREATE TABLE system_info (
    id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    ip VARCHAR(45) NOT NULL,
    cpu_cores INT NOT NULL,
    cpu_model_name VARCHAR(255) NOT NULL,
    cpu_mhz FLOAT NOT NULL,
    cpu_cache_size INT NOT NULL,
    cpu_usage FLOAT NOT NULL,
    memory_total BIGINT UNSIGNED NOT NULL,
    memory_available BIGINT UNSIGNED NOT NULL,
    memory_used BIGINT UNSIGNED NOT NULL,
    memory_used_percent FLOAT NOT NULL,
    disk_device VARCHAR(255) NOT NULL,
    disk_total BIGINT UNSIGNED NOT NULL,
    disk_free BIGINT UNSIGNED NOT NULL,
    disk_used BIGINT UNSIGNED NOT NULL,
    disk_used_percent FLOAT NOT NULL,
    network_interface_name VARCHAR(255) NOT NULL,
    network_bytes_sent BIGINT UNSIGNED NOT NULL,
    network_bytes_recv BIGINT UNSIGNED NOT NULL,
    network_packets_sent BIGINT UNSIGNED NOT NULL,
    network_packets_recv BIGINT UNSIGNED NOT NULL,
    hostname VARCHAR(255) NOT NULL,
    os VARCHAR(255) NOT NULL,
    platform VARCHAR(255) NOT NULL,
    platform_version VARCHAR(255) NOT NULL,
    uptime BIGINT UNSIGNED NOT NULL,
    load1 FLOAT NOT NULL,
    load5 FLOAT NOT NULL,
    load15 FLOAT NOT NULL,
    timestamp DATETIME NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
);

CREATE TABLE region_probe_info (
    id INT AUTO_INCREMENT PRIMARY KEY,
    source_ip VARCHAR(15) NOT NULL,
    source_region VARCHAR(50) NOT NULL,
    target_ip VARCHAR(15) NOT NULL,
    target_region VARCHAR(50) NOT NULL,
    tcp_delay INT NOT NULL,
    probe_time DATETIME NOT NULL
);

CREATE TABLE network_metrics (
    id INT AUTO_INCREMENT PRIMARY KEY,     
    source_ip VARCHAR(15) NOT NULL,          
    destination_ip VARCHAR(15) NOT NULL,      
    link_latency FLOAT NOT NULL,           
    cpu_mean FLOAT NOT NULL,              
    cpu_variance FLOAT NOT NULL,          
    virtual_queue_cpu_mean FLOAT NOT NULL,      
    virtual_queue_cpu_variance FLOAT NOT NULL,  
    C INT,                                    
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP -- 
);

CREATE TABLE domain_origin (
    domain VARCHAR(20) PRIMARY KEY,
    origin_ip VARCHAR(20) NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP -- 
);

CREATE TABLE node_region (
    id INT AUTO_INCREMENT PRIMARY KEY,
    ip VARCHAR(50) NOT NULL UNIQUE,
    region VARCHAR(50) NOT NULL,
    hostname VARCHAR(100),
    description VARCHAR(255),
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE domain_config (
    id INT AUTO_INCREMENT PRIMARY KEY,
    domain_name VARCHAR(255) NOT NULL UNIQUE,
    total_req_increment INT NOT NULL,
    redistribution_proportion DOUBLE NOT NULL,
    last_updated TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
);