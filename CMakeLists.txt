cmake_minimum_required(VERSION 3.0)

project(fb2converter NONE)

message(STATUS "Running cmake version ${CMAKE_VERSION}")
if (NOT CMAKE_BUILD_TYPE)
    set(CMAKE_BUILD_TYPE "Release" CACHE STRING "default to release build type" FORCE)
endif()
set_property(CACHE CMAKE_BUILD_TYPE PROPERTY STRINGS "Debug" "Release")
message(STATUS "Build type: ${CMAKE_BUILD_TYPE}")
message(STATUS "Generator: ${CMAKE_GENERATOR}")

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
    set(DEST_DIR "${PROJECT_SOURCE_DIR}/bin_${MSYSTEM_NAME}")
else()
    set(KINDLEGEN "kindlegen/linux/kindlegen")
    message(STATUS "Targeting Linux executable")
    set(DEST_DIR "${PROJECT_SOURCE_DIR}/bin_linux")
endif()
message(STATUS "Installation directory: ${DEST_DIR}")

find_package(Go 1.11 REQUIRED)
find_package(Git REQUIRED)

# Project version number
set(PRJ_VERSION_MAJOR 1)
set(PRJ_VERSION_MINOR 4)

########################################################################################################
# Setting up versioning
########################################################################################################

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
configure_file("${PROJECT_SOURCE_DIR}/cmake/version.go.in" "${PROJECT_SOURCE_DIR}/src/fb2converter/misc/version.go")

# TODO: do we want to distribute history?
# execute_process(COMMAND ${CMAKE_COMMAND} -E env ${GIT_EXECUTABLE} changelog --final-tag ${PRJ_VERSION_MAJOR}.${PRJ_VERSION_MINOR} --stdout OUTPUT_FILE ${PROJECT_BINARY_DIR}/history.txt ERROR_QUIET)

message(STATUS "Building version \"${PRJ_VERSION_MAJOR}.${PRJ_VERSION_MINOR}\" git \"${GIT_HASH}\"")

if(CMAKE_BUILD_TYPE STREQUAL "Debug")
    # Create git pre-commit hook to check formatting before commit
    if(EXISTS "${PROJECT_SOURCE_DIR}/.git" AND IS_DIRECTORY "${PROJECT_SOURCE_DIR}/.git")
        execute_process(COMMAND ${CMAKE_COMMAND} -E copy ${PROJECT_SOURCE_DIR}/cmake/pre-commit ${PROJECT_SOURCE_DIR}/.git/hooks/. WORKING_DIRECTORY ${PROJECT_SOURCE_DIR})
    endif()
    message(STATUS "Vendoring for go")
    if(NOT EXISTS ${PROJECT_SOURCE_DIR}/src/fb2converter/go.mod)
        message(FATAL_ERROR "Unable to find go.mod. Exiting...")
    endif()
    execute_process(COMMAND ${GO_EXECUTABLE} mod download WORKING_DIRECTORY "${PROJECT_SOURCE_DIR}/src/fb2converter")
endif()
if(EXISTS "${PROJECT_SOURCE_DIR}/${KINDLEGEN}")
    execute_process(COMMAND ${CMAKE_COMMAND} -E copy ${PROJECT_SOURCE_DIR}/${KINDLEGEN} ${PROJECT_BINARY_DIR}/.)
endif()

########################################################################################################
# main target - always out of date, "go build" will figure it out
########################################################################################################

add_custom_target(bin_fb2c ALL
    DEPENDS ${PROJECT_SOURCE_DIR}/src/fb2converter/processor/enums_string.go
        ${PROJECT_SOURCE_DIR}/src/fb2converter/static/files.go
    COMMAND GOPATH=${PROJECT_SOURCE_DIR} GOOS=${CROSS_GOOS} GOARCH=${CROSS_GOARCH}
        ${GO_EXECUTABLE} build -o ${PROJECT_BINARY_DIR}/fb2c${CMAKE_EXECUTABLE_SUFFIX}
        -ldflags='-X main.LastGitCommit=${GIT_HASH}'
        fb2converter/cmd/fb2c
    COMMENT "Building fb2c..."
    WORKING_DIRECTORY "${PROJECT_SOURCE_DIR}/src/fb2converter")

if(CROSS_GOOS STREQUAL "windows")
    install(PROGRAMS ${PROJECT_BINARY_DIR}/fb2c${CMAKE_EXECUTABLE_SUFFIX} DESTINATION ${DEST_DIR}/fb2converter CONFIGURATIONS Release)
    install(PROGRAMS ${PROJECT_SOURCE_DIR}/${KINDLEGEN} DESTINATION ${DEST_DIR}/fb2converter CONFIGURATIONS Release)

    # myhomelib suppport - Windows only
    add_custom_target(bin_fb2mobi ALL
        COMMAND GOPATH=${PROJECT_SOURCE_DIR} GOOS=${CROSS_GOOS} GOARCH=${CROSS_GOARCH}
            ${GO_EXECUTABLE} build -o ${PROJECT_BINARY_DIR}/fb2mobi${CMAKE_EXECUTABLE_SUFFIX}
            -ldflags='-X main.LastGitCommit=${GIT_HASH}'
            fb2converter/cmd/fb2mobi
        COMMENT "Building fb2mobi..."
        WORKING_DIRECTORY "${PROJECT_SOURCE_DIR}/src/fb2converter")
    install(PROGRAMS ${PROJECT_BINARY_DIR}/fb2mobi${CMAKE_EXECUTABLE_SUFFIX} DESTINATION ${DEST_DIR}/fb2mobi CONFIGURATIONS Release)
    add_custom_target(bin_fb2epub ALL
        COMMAND GOPATH=${PROJECT_SOURCE_DIR} GOOS=${CROSS_GOOS} GOARCH=${CROSS_GOARCH}
            ${GO_EXECUTABLE} build -o ${PROJECT_BINARY_DIR}/fb2epub${CMAKE_EXECUTABLE_SUFFIX}
            -ldflags='-X main.LastGitCommit=${GIT_HASH}'
            fb2converter/cmd/fb2epub
        COMMENT "Building fb2epub..."
        WORKING_DIRECTORY "${PROJECT_SOURCE_DIR}/src/fb2converter")
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

if(CMAKE_BUILD_TYPE STREQUAL "Debug")
    set(TEST_RUN_ARGS "-v")
else()
    set(TEST_RUN_ARGS "")
endif()

# add_custom_target(test_all
#     DEPENDS test_processor test_hyphenator
#     WORKING_DIRECTORY ${PROJECT_SOURCE_DIR}/src/dimple
#     COMMENT "Running ALL tests...")

add_custom_target(test_processor
    DEPENDS ${PROJECT_SOURCE_DIR}/src/fb2converter/processor/enums_string.go
        ${PROJECT_SOURCE_DIR}/src/fb2converter/static/files.go
    COMMAND GOPATH=${PROJECT_SOURCE_DIR}
        ${GO_EXECUTABLE} test ${TEST_RUN_ARGS}
            -coverprofile=${PROJECT_BINARY_DIR}/test_processor.out
            fb2converter/processor
    COMMENT "processor tests..."
    WORKING_DIRECTORY "${PROJECT_SOURCE_DIR}/src/fb2converter")

add_custom_target(test_hyphenator
    DEPENDS ${PROJECT_SOURCE_DIR}/src/fb2converter/processor/enums_string.go
        ${PROJECT_SOURCE_DIR}/src/fb2converter/static/files.go
    COMMAND GOPATH=${PROJECT_SOURCE_DIR}
        ${GO_EXECUTABLE} test ${TEST_RUN_ARGS}
            -coverprofile=${PROJECT_BINARY_DIR}/test_hyphenator.out
            fb2converter/hyphenator
    COMMENT "hyphenator tests..."
    WORKING_DIRECTORY "${PROJECT_SOURCE_DIR}/src/fb2converter")

########################################################################################################
# Development
########################################################################################################

if(CMAKE_BUILD_TYPE STREQUAL "Debug")

    # main target with race detector
    add_custom_target(bin_fb2c_race
        DEPENDS ${PROJECT_SOURCE_DIR}/src/fb2converter/processor/enums_string.go
            ${PROJECT_SOURCE_DIR}/src/fb2converter/static/files.go
        COMMAND GOPATH=${PROJECT_SOURCE_DIR} GOOS=${CROSS_GOOS} GOARCH=${CROSS_GOARCH}
            ${GO_EXECUTABLE} build -race -o ${PROJECT_BINARY_DIR}/fb2c_race${CMAKE_EXECUTABLE_SUFFIX}
            -ldflags='-X main.LastGitCommit=${GIT_HASH}'
            fb2converter/cmd/fb2c
        WORKING_DIRECTORY "${PROJECT_SOURCE_DIR}/src/fb2converter"
        COMMENT "Building fb2c with race detector...")

    ########################################################################################################
    # Code generation - on host platform only
    ########################################################################################################

    add_custom_command(OUTPUT ${PROJECT_BINARY_DIR}/stringer
        COMMAND GOPATH=${PROJECT_SOURCE_DIR}
            ${GO_EXECUTABLE} build -o ${PROJECT_BINARY_DIR}/stringer
            golang.org/x/tools/cmd/stringer
        WORKING_DIRECTORY "${PROJECT_SOURCE_DIR}/src/fb2converter")
    add_custom_target(bin_stringer
        DEPENDS ${PROJECT_BINARY_DIR}/stringer
        COMMENT "Building stringer executable...")

    add_custom_command(OUTPUT ${PROJECT_SOURCE_DIR}/src/fb2converter/processor/enums_string.go
        DEPENDS ${PROJECT_BINARY_DIR}/stringer
            ${PROJECT_SOURCE_DIR}/src/fb2converter/processor/enums.go
        COMMAND GOPATH=${PROJECT_SOURCE_DIR}
        ${PROJECT_BINARY_DIR}/stringer -linecomment -type OutputFmt,NotesFmt,TOCPlacement,TOCType,APNXGeneration,StampPlacement
                -output ${PROJECT_SOURCE_DIR}/src/fb2converter/processor/enums_string.go
                ${PROJECT_SOURCE_DIR}/src/fb2converter/processor/enums.go
        WORKING_DIRECTORY "${PROJECT_SOURCE_DIR}/src/fb2converter"
        COMMENT "Generating stringer interfaces")

    add_custom_command(OUTPUT ${PROJECT_BINARY_DIR}/go-bindata
        COMMAND GOPATH=${PROJECT_SOURCE_DIR}
            ${GO_EXECUTABLE} build -o ${PROJECT_BINARY_DIR}/go-bindata
            github.com/shuLhan/go-bindata/cmd/go-bindata
        WORKING_DIRECTORY "${PROJECT_SOURCE_DIR}/src/fb2converter")
    add_custom_target(bin_go-bindata
        DEPENDS ${PROJECT_BINARY_DIR}/go-bindata
        COMMENT "Building go-bindata executable...")

    file(GLOB_RECURSE SRCS_STATIC LIST_DIRECTORIES false ${PROJECT_SOURCE_DIR}/static/*)
    add_custom_command(OUTPUT ${PROJECT_SOURCE_DIR}/src/fb2converter/static/files.go
        DEPENDS ${PROJECT_BINARY_DIR}/go-bindata ${SRCS_STATIC}
        # -nocompress
        COMMAND GOPATH=${PROJECT_SOURCE_DIR}
            ${PROJECT_BINARY_DIR}/go-bindata
                -prefix ${PROJECT_SOURCE_DIR}/static/
                -pkg static
                -o static/files.go
                ${PROJECT_SOURCE_DIR}/static/...
        # cannot do this due to the size COMMAND GOPATH=${PROJECT_SOURCE_DIR} ${GO_EXECUTABLE} fmt static/files.go
        WORKING_DIRECTORY ${PROJECT_SOURCE_DIR}/src/fb2converter
        COMMENT "Generating Go resources for static assets")

    ########################################################################################################
    # Go dependency management
    ########################################################################################################

    add_custom_target(deps_update
        SOURCES ${PROJECT_SOURCE_DIR}/src/fb2converter/go.mod
        # This what needs to be done (COMMAND GOPATH=${PROJECT_SOURCE_DIR} ${GO_EXECUTABLE} clean -modcache fb2converter/cmd/fb2c), but removing directory is much faster
        COMMAND ${CMAKE_COMMAND} -E remove_directory ${PROJECT_SOURCE_DIR}/pkg/mod
        COMMAND GOPATH=${PROJECT_SOURCE_DIR} ${GO_EXECUTABLE} mod download
        WORKING_DIRECTORY "${PROJECT_SOURCE_DIR}/src/fb2converter"
        COMMENT "Updating ALL dependencies (go mod download)"
        VERBATIM)

    add_custom_target(deps_print
        SOURCES ${PROJECT_SOURCE_DIR}/src/fb2converter/go.mod
        COMMAND GOPATH=${PROJECT_SOURCE_DIR} ${GO_EXECUTABLE} mod graph
        WORKING_DIRECTORY "${PROJECT_SOURCE_DIR}/src/fb2converter"
        COMMENT "Printing dependencies (go mod graph)"
        VERBATIM)

    add_custom_target(deps_tidy
        SOURCES ${PROJECT_SOURCE_DIR}/src/fb2converter/go.mod
        COMMAND GOPATH=${PROJECT_SOURCE_DIR} ${GO_EXECUTABLE} mod tidy
        WORKING_DIRECTORY "${PROJECT_SOURCE_DIR}/src/fb2converter"
        COMMENT "Printing dependencies (go mod tidy)"
        VERBATIM)

    add_custom_target(deps_verify
        SOURCES ${PROJECT_SOURCE_DIR}/src/fb2converter/go.mod
        COMMAND GOPATH=${PROJECT_SOURCE_DIR} ${GO_EXECUTABLE} mod verify
        WORKING_DIRECTORY "${PROJECT_SOURCE_DIR}/src/fb2converter"
        COMMENT "Printing dependencies (go mod verify)"
        VERBATIM)

    add_custom_target(deps_vendor
        SOURCES ${PROJECT_SOURCE_DIR}/src/fb2converter/go.mod
        COMMAND GOPATH=${PROJECT_SOURCE_DIR} ${GO_EXECUTABLE} mod vendor
        WORKING_DIRECTORY "${PROJECT_SOURCE_DIR}/src/fb2converter"
        COMMENT "Creating vendor directory (go mod vendor)"
        VERBATIM)
endif()
