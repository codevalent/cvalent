package com.example;

import javax.annotation.Nullable;
import java.util.Optional;

public class UserService {
    public Optional<User> findUser(String id) {
        return Optional.empty();
    }

    public void updateUser(@Nullable String name, int age) {
    }
}
