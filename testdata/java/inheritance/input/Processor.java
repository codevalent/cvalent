package com.example;

public interface Processor {
    Result process(Input input);
    void cleanup();
}
