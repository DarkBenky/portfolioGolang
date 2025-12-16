<template>
  <v-container class="fill-height" fluid>
    <v-row align="center" justify="center">
      <v-col cols="12" sm="8" md="4">
        <v-card class="elevation-12">
          <v-toolbar color="primary" dark flat>
            <v-toolbar-title>{{ isLogin ? 'Login' : 'Register' }}</v-toolbar-title>
          </v-toolbar>

          <v-card-text>
            <v-form ref="form" v-model="valid" @submit.prevent="submit">
              <!-- Username (only for register) -->
              <v-text-field
                v-if="!isLogin"
                v-model="username"
                label="Username"
                prepend-icon="mdi-account"
                :rules="usernameRules"
                required
              ></v-text-field>

              <!-- Email -->
              <v-text-field
                v-model="email"
                label="Email"
                prepend-icon="mdi-email"
                type="email"
                :rules="emailRules"
                required
              ></v-text-field>

              <!-- Password -->
              <v-text-field
                v-model="password"
                label="Password"
                prepend-icon="mdi-lock"
                :type="showPassword ? 'text' : 'password'"
                :append-icon="showPassword ? 'mdi-eye' : 'mdi-eye-off'"
                @click:append="showPassword = !showPassword"
                :rules="passwordRules"
                required
              ></v-text-field>

              <!-- Confirm Password (only for register) -->
              <v-text-field
                v-if="!isLogin"
                v-model="confirmPassword"
                label="Confirm Password"
                prepend-icon="mdi-lock-check"
                :type="showPassword ? 'text' : 'password'"
                :rules="confirmPasswordRules"
                required
              ></v-text-field>

              <!-- Error message -->
              <v-alert
                v-if="errorMessage"
                type="error"
                variant="tonal"
                class="mb-4"
              >
                {{ errorMessage }}
              </v-alert>

              <!-- Success message -->
              <v-alert
                v-if="successMessage"
                type="success"
                variant="tonal"
                class="mb-4"
              >
                {{ successMessage }}
              </v-alert>
            </v-form>
          </v-card-text>

          <v-card-actions>
            <v-spacer></v-spacer>
            <v-btn
              color="primary"
              variant="elevated"
              :loading="loading"
              :disabled="!valid"
              @click="submit"
            >
              {{ isLogin ? 'Login' : 'Register' }}
            </v-btn>
          </v-card-actions>

          <v-divider></v-divider>

          <v-card-text class="text-center">
            <span>{{ isLogin ? "Don't have an account?" : 'Already have an account?' }}</span>
            <v-btn
              variant="text"
              color="primary"
              @click="toggleMode"
            >
              {{ isLogin ? 'Register' : 'Login' }}
            </v-btn>
          </v-card-text>
        </v-card>
      </v-col>
    </v-row>
  </v-container>
</template>

<script setup>
import { ref, computed } from 'vue'
import { API_BASE_URL } from '../config'

const API_URL = API_BASE_URL

// Form state
const isLogin = ref(true)
const valid = ref(false)
const loading = ref(false)
const showPassword = ref(false)
const errorMessage = ref('')
const successMessage = ref('')

// Form fields
const username = ref('')
const email = ref('')
const password = ref('')
const confirmPassword = ref('')

// Validation rules
const usernameRules = [
  v => !!v || 'Username is required',
  v => v.length >= 3 || 'Username must be at least 3 characters'
]

const emailRules = [
  v => !!v || 'Email is required',
  v => /.+@.+\..+/.test(v) || 'Email must be valid'
]

const passwordRules = [
  v => !!v || 'Password is required',
  v => v.length >= 6 || 'Password must be at least 6 characters'
]

const confirmPasswordRules = computed(() => [
  v => !!v || 'Please confirm your password',
  v => v === password.value || 'Passwords do not match'
])

// Cookie utilities
function setCookie(name, value, days) {
  const expires = new Date()
  expires.setTime(expires.getTime() + days * 24 * 60 * 60 * 1000)
  document.cookie = `${name}=${value};expires=${expires.toUTCString()};path=/;SameSite=Strict`
}

function getCookie(name) {
  const nameEQ = name + '='
  const cookies = document.cookie.split(';')
  for (let cookie of cookies) {
    cookie = cookie.trim()
    if (cookie.indexOf(nameEQ) === 0) {
      return cookie.substring(nameEQ.length)
    }
  }
  return null
}

function deleteCookie(name) {
  document.cookie = `${name}=;expires=Thu, 01 Jan 1970 00:00:00 UTC;path=/;`
}

// Toggle between login and register
function toggleMode() {
  isLogin.value = !isLogin.value
  errorMessage.value = ''
  successMessage.value = ''
  // Reset form
  username.value = ''
  email.value = ''
  password.value = ''
  confirmPassword.value = ''
}

// Submit form
async function submit() {
  if (!valid.value) return

  loading.value = true
  errorMessage.value = ''
  successMessage.value = ''

  try {
    const endpoint = isLogin.value ? '/login' : '/register'
    const body = isLogin.value
      ? { email: email.value, password: password.value }
      : { user_name: username.value, email: email.value, password: password.value }

    const response = await fetch(`${API_URL}${endpoint}`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json'
      },
      body: JSON.stringify(body)
    })

    const data = await response.json()

    if (!response.ok) {
      throw new Error(data.error || data.message || 'An error occurred')
    }

    if (isLogin.value) {
      // Save token to cookie with 90 days expiration
      setCookie('auth_token', data.token, 90)
      setCookie('user_email', data.email, 90)
      successMessage.value = 'Login successful! Redirecting...'
      
      // Emit event or redirect
      setTimeout(() => {
        window.location.href = '/'
      }, 1000)
    } else {
      successMessage.value = 'Registration successful! Please login.'
      // Switch to login mode
      setTimeout(() => {
        toggleMode()
      }, 1500)
    }
  } catch (error) {
    errorMessage.value = error.message || 'An error occurred'
  } finally {
    loading.value = false
  }
}

// Check if already logged in
function checkAuth() {
  const token = getCookie('auth_token')
  if (token) {
    // User is already logged in
    return true
  }
  return false
}

// Expose for parent components
defineExpose({
  getCookie,
  setCookie,
  deleteCookie,
  checkAuth
})
</script>

<style scoped>
.fill-height {
  min-height: 100vh;
}
</style>
