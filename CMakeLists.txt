cmake_minimum_required(VERSION 3.0)

project(fb2converter NONE)

message(STATUS "Running cmake version ${CMAKE_VERSION}")
if (NOT CMAKE_BUILD_TYPE)
    set(CMAKE_BUILD_TYPE "Release" CACHE STRING "default to release build type" FORCE)
endif()
set_property(CACHE CMAKE_BUILD_TYPE PROPERTY STRINGS "Debug" "Release")
message(STATUS "Build type: ${CMAKE_BUILD_TYPE}")
message(STATUS "Generator: ${CMAKE_GENERATOR}")

if(CMAKE_BUILD_TYPE STREQUAL "Debug")
    message(STATUS "********************************************************************************")
    message(STATUS "*** Do not forget to do \"make deps_vendor\" before switching to Release mode! ***")
    message(STATUS "********************************************************************************")
endif()

set(CMAKE_MODULE_PATH ${CMAKE_MODULE_PATH} ${CMAKE_SOURCE_DIR}/cmake)

# Release installation directory
if(CMAKE_CROSSCOMPILING)
    if(MSYSTEM_NAME STREQUAL "win32")
        set(KINDLEGEN "kindlegen/windows/kindlegen.exe")
        set(CROSS_GOOS windows)
        set(CROSS_GOARCH 386)
        message(STATUS "Targeting 32 bits Windows executable")
    elseif(MSYSTEM_NAME STREQUAL "win64")
        set(KINDLEGEN "kindlegen/windows/kindlegen.exe")
        set(CROSS_GOOS windows)
        set(CROSS_GOARCH amd64)
        message(STATUS "Targeting 64 bits Windows executable")
    elseif(MSYSTEM_NAME STREQUAL "darwin")
        set(KINDLEGEN "kindlegen/macos/kindlegen")
        set(CROSS_GOOS darwin)
        set(CROSS_GOARCH amd64)
        message(STATUS "Targeting 64 bits Darwin executable")
    else()
        message(FATAL_ERROR "Currently unsuppored environment \"${MSYSTEM_NAME}\". CMake willl exit now.")
    endif()
else()
    if (NOT "$ENV{MSYSTEM_NAME}" STREQUAL "")
        set(MSYSTEM_NAME "$ENV{MSYSTEM_NAME}" CACHE INTERNAL "Copied from environment")
    endif()
    if("$ENV{MSYSTEM_NAME}" STREQUAL "linux32")
        set(KINDLEGEN "kindlegen/linux/kindlegen")
        set(CROSS_GOOS linux)
        set(CROSS_GOARCH 386)
        message(STATUS "Targeting 32 bits Linux executable")
    else()
        # host platform is always linux amd64
        set(KINDLEGEN "kindlegen/linux/kindlegen")
        message(STATUS "Targeting 64 bits Linux executable")
    endif()
endif()
set(DEST_DIR "${PROJECT_SOURCE_DIR}/bin_${MSYSTEM_NAME}")
message(STATUS "Installation directory: ${DEST_DIR}")

find_package(Go 1.12 REQUIRED)
find_package(Git REQUIRED)

# Make sure we setup go properly
if (DEFINED ENV{GOPATH})
    set(GO_PATH "$ENV{GOPATH}")
else()
    set(GO_PATH "/tmp/gopkg_${CMAKE_PROJECT_NAME}")
    execute_process(COMMAND ${CMAKE_COMMAND} -E make_directory ${GO_PATH})
endif()
message(STATUS "Using go modules with GOPATH=${GO_PATH}")
if(GO_PATH MATCHES "^${PROJECT_SOURCE_DIR}.*")
    message(FATAL_ERROR "We are working in \"go modules outside of GOPATH mode\" - GOPATH should be outside of project source directory!")
endif()

########################################################################################################
# Setting up versioning
########################################################################################################

# Project version number
set(PRJ_VERSION_MAJOR 1)
set(PRJ_VERSION_MINOR 20)

if (EXISTS "${PROJECT_SOURCE_DIR}/.git" AND IS_DIRECTORY "${PROJECT_SOURCE_DIR}/.git")
    execute_process(COMMAND ${CMAKE_SOURCE_DIR}/cmake/githash.sh ${GIT_EXECUTABLE}
        WORKING_DIRECTORY "${PROJECT_SOURCE_DIR}"
        RESULT_VARIABLE res
        OUTPUT_VARIABLE GIT_HASH
        ERROR_QUIET
        OUTPUT_STRIP_TRAILING_WHITESPACE)

    # This will cause reconfiguration on every commit
    set_property(DIRECTORY APPEND PROPERTY CMAKE_CONFIGURE_DEPENDS "${PROJECT_SOURCE_DIR}/.git/index")
else()
    set(GIT_HASH "no-git")
endif()
configure_file("${PROJECT_SOURCE_DIR}/cmake/version.go.in" "${PROJECT_SOURCE_DIR}/misc/version.go")

# distribute history
execute_process(COMMAND ${CMAKE_COMMAND} -E env ${GIT_EXECUTABLE} changelog --all --stdout OUTPUT_FILE ${PROJECT_BINARY_DIR}/changelog.txt ERROR_QUIET)
install(FILES ${PROJECT_BINARY_DIR}/changelog.txt DESTINATION ${DEST_DIR} CONFIGURATIONS Release)

message(STATUS "Building version \"${PRJ_VERSION_MAJOR}.${PRJ_VERSION_MINOR}\" git \"${GIT_HASH}\"")

if(CMAKE_BUILD_TYPE STREQUAL "Debug")
    # Create git pre-commit hook to check formatting before commit
    if(EXISTS "${PROJECT_SOURCE_DIR}/.git" AND IS_DIRECTORY "${PROJECT_SOURCE_DIR}/.git")
        execute_process(COMMAND ${CMAKE_COMMAND} -E copy ${PROJECT_SOURCE_DIR}/cmake/pre-commit ${PROJECT_SOURCE_DIR}/.git/hooks/. WORKING_DIRECTORY ${PROJECT_SOURCE_DIR})
    endif()
    execute_process(COMMAND ${CMAKE_COMMAND} -E remove ${PROJECT_SOURCE_DIR}/static/files.go WORKING_DIRECTORY ${PROJECT_SOURCE_DIR})
endif()
if(EXISTS "${PROJECT_SOURCE_DIR}/${KINDLEGEN}")
    execute_process(COMMAND ${CMAKE_COMMAND} -E copy ${PROJECT_SOURCE_DIR}/${KINDLEGEN} ${PROJECT_BINARY_DIR}/.)
endif()

########################################################################################################
# Some contexts dependent settings
########################################################################################################

if(CMAKE_BUILD_TYPE STREQUAL "Debug")
    set(TEST_RUN_ARGS "-v")
else()
    set(GO_MOD_ARGS "-mod=vendor")
endif()

########################################################################################################
# main target - always out of date, "go build" will figure it out
########################################################################################################

add_custom_target(bin_fb2c ALL
    DEPENDS ${PROJECT_SOURCE_DIR}/processor/enums_string.go
        ${PROJECT_SOURCE_DIR}/static/files.go
    COMMAND GOPATH=${GO_PATH} GO111MODULE=on GOOS=${CROSS_GOOS} GOARCH=${CROSS_GOARCH}
        ${GO_EXECUTABLE} build -o ${PROJECT_BINARY_DIR}/fb2c${CMAKE_EXECUTABLE_SUFFIX}
        -ldflags='-X main.LastGitCommit=${GIT_HASH}'
        ${GO_MOD_ARGS}
    COMMENT "Building fb2c..."
    WORKING_DIRECTORY "${PROJECT_SOURCE_DIR}")

if(CROSS_GOOS STREQUAL "windows")
    install(PROGRAMS ${PROJECT_BINARY_DIR}/fb2c${CMAKE_EXECUTABLE_SUFFIX} DESTINATION ${DEST_DIR}/fb2converter CONFIGURATIONS Release)
    install(PROGRAMS ${PROJECT_SOURCE_DIR}/${KINDLEGEN} DESTINATION ${DEST_DIR}/fb2converter CONFIGURATIONS Release)

    # myhomelib suppport - Windows only
    add_custom_target(bin_fb2mobi ALL
        COMMAND GOPATH=${GO_PATH} GO111MODULE=on GOOS=${CROSS_GOOS} GOARCH=${CROSS_GOARCH}
            ${GO_EXECUTABLE} build -o ${PROJECT_BINARY_DIR}/fb2mobi${CMAKE_EXECUTABLE_SUFFIX}
            -ldflags='-X main.LastGitCommit=${GIT_HASH}'
            ${GO_MOD_ARGS} fb2converter/cmd/fb2mobi
        COMMENT "Building fb2mobi..."
        WORKING_DIRECTORY "${PROJECT_SOURCE_DIR}")
    install(PROGRAMS ${PROJECT_BINARY_DIR}/fb2mobi${CMAKE_EXECUTABLE_SUFFIX} DESTINATION ${DEST_DIR}/fb2mobi CONFIGURATIONS Release)
    add_custom_target(bin_fb2epub ALL
        COMMAND GOPATH=${GO_PATH} GO111MODULE=on GOOS=${CROSS_GOOS} GOARCH=${CROSS_GOARCH}
            ${GO_EXECUTABLE} build -o ${PROJECT_BINARY_DIR}/fb2epub${CMAKE_EXECUTABLE_SUFFIX}
            -ldflags='-X main.LastGitCommit=${GIT_HASH}'
            ${GO_MOD_ARGS} fb2converter/cmd/fb2epub
        COMMENT "Building fb2epub..."
        WORKING_DIRECTORY "${PROJECT_SOURCE_DIR}")
    install(PROGRAMS ${PROJECT_BINARY_DIR}/fb2epub${CMAKE_EXECUTABLE_SUFFIX} DESTINATION ${DEST_DIR}/fb2epub CONFIGURATIONS Release)
else()
    install(PROGRAMS ${PROJECT_BINARY_DIR}/fb2c${CMAKE_EXECUTABLE_SUFFIX} DESTINATION ${DEST_DIR} CONFIGURATIONS Release)
    install(PROGRAMS ${PROJECT_SOURCE_DIR}/${KINDLEGEN} DESTINATION ${DEST_DIR} CONFIGURATIONS Release)
endif()

########################################################################################################
# Test targets: to test various parts of the system specify multiple test targets
########################################################################################################

# Temporary directory for test results
execute_process(COMMAND ${CMAKE_COMMAND} -E make_directory tests_results)

add_custom_target(test_all
    DEPENDS test_processor test_hyphenator
    WORKING_DIRECTORY ${PROJECT_SOURCE_DIR}/src/dimple
    COMMENT "Running ALL tests...")

add_custom_target(test_processor
    DEPENDS ${PROJECT_SOURCE_DIR}/processor/enums_string.go
        ${PROJECT_SOURCE_DIR}/static/files.go
    COMMAND GOPATH=${GO_PATH} GO111MODULE=on
        ${GO_EXECUTABLE} test ${TEST_RUN_ARGS}
            -coverprofile=${PROJECT_BINARY_DIR}/test_processor.out
            ${GO_MOD_ARGS} fb2converter/processor
    COMMENT "processor tests..."
    WORKING_DIRECTORY "${PROJECT_SOURCE_DIR}")

add_custom_target(test_hyphenator
    DEPENDS ${PROJECT_SOURCE_DIR}/processor/enums_string.go
        ${PROJECT_SOURCE_DIR}/static/files.go
    COMMAND GOPATH=${GO_PATH} GO111MODULE=on
        ${GO_EXECUTABLE} test ${TEST_RUN_ARGS}
            -coverprofile=${PROJECT_BINARY_DIR}/test_hyphenator.out
            ${GO_MOD_ARGS} fb2converter/hyphenator
    COMMENT "hyphenator tests..."
    WORKING_DIRECTORY "${PROJECT_SOURCE_DIR}")

########################################################################################################
# Development
########################################################################################################

if(CMAKE_BUILD_TYPE STREQUAL "Debug")

    # main target with race detector
    add_custom_target(bin_fb2c_race
        DEPENDS ${PROJECT_SOURCE_DIR}/processor/enums_string.go
            ${PROJECT_SOURCE_DIR}/static/files.go
        COMMAND GOPATH=${GO_PATH} GO111MODULE=on GOOS=${CROSS_GOOS} GOARCH=${CROSS_GOARCH}
            ${GO_EXECUTABLE} build -race -o ${PROJECT_BINARY_DIR}/fb2c_race${CMAKE_EXECUTABLE_SUFFIX}
            -ldflags='-X main.LastGitCommit=${GIT_HASH}'
            fb2converter/cmd/fb2c
        WORKING_DIRECTORY "${PROJECT_SOURCE_DIR}"
        COMMENT "Building fb2c with race detector...")

    ########################################################################################################
    # Code generation - on host platform only
    ########################################################################################################

    add_custom_command(OUTPUT ${PROJECT_BINARY_DIR}/stringer
        COMMAND GOPATH=${GO_PATH} GO111MODULE=on
            ${GO_EXECUTABLE} build -o ${PROJECT_BINARY_DIR}/stringer
            golang.org/x/tools/cmd/stringer
        WORKING_DIRECTORY "${PROJECT_SOURCE_DIR}")
    add_custom_target(bin_stringer
        DEPENDS ${PROJECT_BINARY_DIR}/stringer
        COMMENT "Building stringer executable...")

    add_custom_command(OUTPUT ${PROJECT_SOURCE_DIR}/processor/enums_string.go
        DEPENDS ${PROJECT_BINARY_DIR}/stringer
            ${PROJECT_SOURCE_DIR}/processor/enums.go
        COMMAND GOPATH=${GO_PATH} GO111MODULE=on
        ${PROJECT_BINARY_DIR}/stringer -linecomment -type OutputFmt,NotesFmt,TOCPlacement,TOCType,APNXGeneration,StampPlacement
                -output ${PROJECT_SOURCE_DIR}/processor/enums_string.go
                ${PROJECT_SOURCE_DIR}/processor/enums.go
        WORKING_DIRECTORY "${PROJECT_SOURCE_DIR}"
        COMMENT "Generating stringer interfaces")

    add_custom_command(OUTPUT ${PROJECT_BINARY_DIR}/go-bindata
        COMMAND GOPATH=${GO_PATH} GO111MODULE=on
            ${GO_EXECUTABLE} build -o ${PROJECT_BINARY_DIR}/go-bindata
            github.com/shuLhan/go-bindata/cmd/go-bindata
        WORKING_DIRECTORY "${PROJECT_SOURCE_DIR}")
    add_custom_target(bin_go-bindata
        DEPENDS ${PROJECT_BINARY_DIR}/go-bindata
        COMMENT "Building go-bindata executable...")

    file(GLOB_RECURSE SRCS_STATIC LIST_DIRECTORIES false ${PROJECT_SOURCE_DIR}/static/*)
    add_custom_command(OUTPUT ${PROJECT_SOURCE_DIR}/static/files.go
        DEPENDS ${PROJECT_BINARY_DIR}/go-bindata ${SRCS_STATIC}
        # -nocompress
        COMMAND GOPATH=${GO_PATH}
            ${PROJECT_BINARY_DIR}/go-bindata
                -prefix ${PROJECT_SOURCE_DIR}/static/
                -ignore static/files.go
                -pkg static
                -o static/files.go
                ${PROJECT_SOURCE_DIR}/static/...
        # cannot do this due to the size COMMAND GOPATH=${GO_PATH} ${GO_EXECUTABLE} fmt static/files.go
        WORKING_DIRECTORY ${PROJECT_SOURCE_DIR}
        COMMENT "Generating Go resources for static assets")

    ########################################################################################################
    # Go dependency management
    ########################################################################################################

    add_custom_target(deps_print
        SOURCES ${PROJECT_SOURCE_DIR}/go.mod
        COMMAND GOPATH=${GO_PATH} GO111MODULE=on ${GO_EXECUTABLE} mod graph
        WORKING_DIRECTORY "${PROJECT_SOURCE_DIR}"
        COMMENT "Printing dependencies (go mod graph)"
        VERBATIM)

    add_custom_target(deps_tidy
        SOURCES ${PROJECT_SOURCE_DIR}/go.mod
        COMMAND GOPATH=${GO_PATH} GO111MODULE=on ${GO_EXECUTABLE} mod tidy
        WORKING_DIRECTORY "${PROJECT_SOURCE_DIR}"
        COMMENT "Printing dependencies (go mod tidy)"
        VERBATIM)

    add_custom_target(deps_verify
        SOURCES ${PROJECT_SOURCE_DIR}/go.mod
        COMMAND GOPATH=${GO_PATH} GO111MODULE=on ${GO_EXECUTABLE} mod verify
        WORKING_DIRECTORY "${PROJECT_SOURCE_DIR}"
        COMMENT "Printing dependencies (go mod verify)"
        VERBATIM)

    add_custom_target(deps_vendor
        SOURCES ${PROJECT_SOURCE_DIR}/go.mod
        COMMAND GOPATH=${GO_PATH} GO111MODULE=on ${GO_EXECUTABLE} mod vendor
        WORKING_DIRECTORY "${PROJECT_SOURCE_DIR}"
        COMMENT "Creating vendor directory (go mod vendor)"
        VERBATIM)
endif()
