package com.example;

public class Config {
    private String host;
    private int port;

    public Config(String host, int port) {
        this.host = host;
        this.port = port;
    }

    public static Config defaultConfig() {
        return new Config("localhost", 8080);
    }

    public String getHost() {
        return host;
    }
}
